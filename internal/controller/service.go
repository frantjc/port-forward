package controller

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/frantjc/port-forward/internal/extip"
	"github.com/frantjc/port-forward/internal/portfwd"
	"github.com/frantjc/port-forward/internal/upnp"
	xslice "github.com/frantjc/x/slice"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ServiceReconciler struct {
	portfwd.PortForwarder
	extip.ExternalIPAddressGetter
	client.Client
	record.EventRecorder
}

const (
	Finalzier                   = "pf.frantj.cc/finalizer"
	ForwardAnnotation           = "pf.frantj.cc/forward"
	PortMapAnnotation           = "pf.frantj.cc/port-map"
	UPnPRemoteHostAnnotation    = "upnp.pf.frantj.cc/remote-host"
	UPnPEnabledAnnotation       = "upnp.pf.frantj.cc/enabled"
	UPnPDescriptionAnnotation   = "upnp.pf.frantj.cc/description"
	UPnPLeaseDurationAnnotation = "upnp.pf.frantj.cc/lease-duration"
)

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=services/status,verbs=update
//+kubebuilder:rbac:groups="",resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_            = logr.FromContextOrDiscard(ctx)
		service      = &corev1.Service{}
		requeueAfter = time.Minute * 15
		portMap      = map[int32]int32{}
	)

	if err := r.Client.Get(ctx, req.NamespacedName, service); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	externalIPAddress, err := r.ExternalIPAddressGetter.GetExternalIPAddress(ctx)
	if err != nil || externalIPAddress == nil {
		errS := "nil IP address"
		if err != nil {
			errS = err.Error()
		}

		r.Eventf(service, corev1.EventTypeWarning, "GetExternalIPAddress", "get external IP address failed with: %s", errS)

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if !service.GetDeletionTimestamp().IsZero() {
		if controllerutil.RemoveFinalizer(service, Finalzier) {
			// TODO: Delete the PortMapping. Not of the utmost importance due to
			// the forced lease duration automatically expiring it at some point.

			lenIngress := len(service.Status.LoadBalancer.Ingress)

			service.Status.LoadBalancer.Ingress = xslice.Filter(service.Status.LoadBalancer.Ingress, func(ingress corev1.LoadBalancerIngress, _ int) bool {
				return !externalIPAddress.Equal(net.ParseIP(ingress.IP))
			})

			if newLenIngress := len(service.Status.LoadBalancer.Ingress); lenIngress > newLenIngress {
				if err := r.Client.Status().Update(ctx, service); err != nil {
					return ctrl.Result{}, err
				}
			}

			if err := r.Client.Update(ctx, service); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	forward, ok := service.Annotations[ForwardAnnotation]
	if !ok {
		return ctrl.Result{}, nil
	}

	if !IsTruthy(forward) {
		r.Eventf(service, corev1.EventTypeNormal, "InvalidAnnotation", "redundant falsy value %s in %s annotation", forward, ForwardAnnotation)
		return ctrl.Result{}, nil
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		r.Eventf(service, corev1.EventTypeWarning, "InvalidAnnotation", "invalid truthy value %s in %s annotation on Service of type %s", forward, ForwardAnnotation, service.Spec.Type)
		return ctrl.Result{}, nil
	}

	if len(service.Status.LoadBalancer.Ingress) == 0 {
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	if pm, ok := service.Annotations[PortMapAnnotation]; ok {
		for _, ports := range strings.Split(pm, ",") {
			var (
				portsSplit    = strings.SplitN(ports, ":", 2)
				lenPortsSplit = len(portsSplit)
			)
			if lenPortsSplit != 2 {
				r.Eventf(service, corev1.EventTypeWarning, "InvalidAnnotation", "invalid entry %s in %s annotation", ports, PortMapAnnotation)
				return ctrl.Result{}, nil
			}

			external, _ := strconv.Atoi(portsSplit[0])

			internal, err := strconv.Atoi(portsSplit[1])
			if err != nil {
				r.Eventf(service, corev1.EventTypeWarning, "InvalidAnnotation", "invalid entry %s in %s annotation", ports, PortMapAnnotation)
			} else {
				portMap[int32(internal)] = int32(external)
			}
		}
	}

	leaseDurationS, ok := service.Annotations[UPnPLeaseDurationAnnotation]
	leaseDuration := requeueAfter * 2
	if ok {
		var err error
		leaseDuration, err = time.ParseDuration(leaseDurationS)
		if err != nil {
			r.Eventf(service, corev1.EventTypeWarning, "InvalidAnnotation", "using default lease duration %s due to invalid duration %s in %s annotation", leaseDuration, leaseDurationS, UPnPLeaseDurationAnnotation)
		} else {
			requeueAfter = leaseDuration / 2
		}
	}

	for _, port := range service.Spec.Ports {
		if xslice.Includes([]corev1.Protocol{corev1.ProtocolTCP, corev1.ProtocolUDP}, port.Protocol) {
			externalPort, ok := portMap[port.Port]
			if !ok {
				externalPort = port.Port
			} else if externalPort <= 0 {
				continue
			}

			portName := xslice.Coalesce(port.Name, fmt.Sprint(port.Port))

			description, ok := service.Annotations[UPnPDescriptionAnnotation]
			if !ok {
				description = fmt.Sprintf(
					"port-forward %s/%s port %s",
					service.ObjectMeta.Namespace, service.ObjectMeta.Name, portName,
				)
			}

			enabled, ok := service.Annotations[UPnPEnabledAnnotation]

			for _, ingress := range service.Status.LoadBalancer.Ingress {
				if err := r.AddPortMapping(ctx, &upnp.PortMapping{
					RemoteHost:     service.Annotations[UPnPRemoteHostAnnotation],
					ExternalPort:   externalPort,
					Protocol:       upnp.Protocol(port.Protocol),
					InternalPort:   port.Port,
					InternalClient: net.ParseIP(ingress.IP),
					Enabled:        !ok || IsTruthy(enabled),
					Description:    description,
					LeaseDuration:  leaseDuration,
				}); err != nil {
					r.Eventf(service, corev1.EventTypeWarning, "AddPortMapping", "map %d to %s:%d for port %s failed with: %s", externalPort, ingress.IP, port.Port, portName, err.Error())
				} else {
					r.Eventf(service, corev1.EventTypeNormal, "AddPortMapping", "mapped %d to %s:%d for port %s", externalPort, ingress.IP, port.Port, portName)
				}
			}
		} else {
			r.Eventf(service, corev1.EventTypeNormal, "UnsupportedProtocol", "skipping UPnP for port with unsupported protocol %s", port.Protocol)
		}
	}

	if controllerutil.AddFinalizer(service, Finalzier) {
		if err := r.Client.Update(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
	}

	var hasIngress bool
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if externalIPAddress.Equal(net.ParseIP(ingress.IP)) {
			hasIngress = true
			break
		}
	}

	if !hasIngress {
		service.Status.LoadBalancer.Ingress = append(service.Status.LoadBalancer.Ingress, corev1.LoadBalancerIngress{
			IP: externalIPAddress.String(),
		})

		if err := r.Client.Status().Update(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func IsTruthy(s string) bool {
	return xslice.Some([]string{"yes", "y", "1", "true"}, func(truthy string, _ int) bool {
		return strings.EqualFold(s, truthy)
	})
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("portfwd")

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
