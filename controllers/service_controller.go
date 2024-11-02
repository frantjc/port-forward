package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frantjc/port-forward/internal/portfwd"
	"github.com/frantjc/port-forward/internal/svcip"
	"github.com/frantjc/port-forward/internal/upnp"
	xslice "github.com/frantjc/x/slice"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type ServiceReconciler struct {
	svcip.ServiceIPAddressGetter
	portfwd.PortForwarder
	client.Client
	record.EventRecorder
}

const (
	Finalizer                   = "pf.frantj.cc/finalizer"
	AnnotationForward           = "pf.frantj.cc/forward"
	AnnotationPortMap           = "pf.frantj.cc/port-map"
	AnnotationEnabled           = "pf.frantj.cc/enabled"
	AnnotationDescription       = "pf.frantj.cc/description"
	AnnotationUPnPRemoteHost    = "upnp.pf.frantj.cc/remote-host"
	AnnotationUPnPLeaseDuration = "upnp.pf.frantj.cc/lease-duration"
)

const (
	EventReasonAnnotation = "PortForwardAnnotation"
	EventReasonForward    = "PortForward"
)

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_              = logr.FromContextOrDiscard(ctx)
		service        = &corev1.Service{}
		requeueAfter   = time.Minute * 15
		portMap        = map[int32]int32{}
		tmpPortNameMap = map[string]any{}
		portNameMap    = map[string]int32{}
		cleanup        = func() (ctrl.Result, error) {
			// TODO: Delete any forwarded ports. Not of the utmost importance due to
			// the forced lease duration automatically expiring it at some point in UPnP,
			// but may become important in future implementations.

			if controllerutil.RemoveFinalizer(service, Finalizer) {
				if err := r.Client.Update(ctx, service); err != nil {
					return ctrl.Result{Requeue: true}, nil
				}
			}

			return ctrl.Result{}, nil
		}
	)

	if err := r.Client.Get(ctx, req.NamespacedName, service); err != nil {
		return ctrl.Result{Requeue: errors.IsNotFound(err)}, client.IgnoreNotFound(err)
	}

	if !service.GetDeletionTimestamp().IsZero() {
		return cleanup()
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		r.Eventf(service, corev1.EventTypeWarning, EventReasonAnnotation, "cannot port forward to Service of type %s", service.Spec.Type)
		return cleanup()
	}

	for _, port := range service.Spec.Ports {
		tmpPortNameMap[port.Name] = struct{}{}
	}

	if pm, ok := service.Annotations[AnnotationPortMap]; ok {
		for _, ports := range strings.Split(pm, ",") {
			var (
				portsSplit    = strings.SplitN(ports, ":", 2)
				lenPortsSplit = len(portsSplit)
			)
			if lenPortsSplit != 2 {
				r.Eventf(service, corev1.EventTypeWarning, EventReasonAnnotation, "invalid entry %s in %s annotation", ports, AnnotationPortMap)
				return ctrl.Result{}, nil
			}

			external, _ := strconv.Atoi(portsSplit[0])

			internal, err := strconv.Atoi(portsSplit[1])
			if err != nil {
				if _, ok := tmpPortNameMap[portsSplit[1]]; ok {
					portNameMap[portsSplit[1]] = int32(external)
				} else {
					r.Eventf(service, corev1.EventTypeWarning, EventReasonAnnotation, "invalid entry %s in %s annotation", ports, AnnotationPortMap)
				}
			} else {
				portMap[int32(internal)] = int32(external)
			}
		}
	}

	var (
		defaultLeaseDuration = requeueAfter * 2
		leaseDuration        = defaultLeaseDuration
	)
	if leaseDurationS, ok := service.Annotations[AnnotationUPnPLeaseDuration]; ok {
		var err error
		leaseDuration, err = time.ParseDuration(leaseDurationS)
		if err != nil {
			leaseDuration = defaultLeaseDuration
			r.Eventf(service, corev1.EventTypeWarning, EventReasonAnnotation, "using default lease duration %s due to invalid duration %s in %s annotation", leaseDuration, leaseDurationS, AnnotationUPnPLeaseDuration)
		} else {
			requeueAfter = leaseDuration / 2
		}
	}

	if ipAddresses := r.ServiceIPAddressGetter.GetServiceIPAddresses(service); len(ipAddresses) > 0 {
		for _, port := range service.Spec.Ports {
			portName := xslice.Coalesce(port.Name, fmt.Sprint(port.Port))

			externalPort, ok := portMap[port.Port]
			if !ok {
				if anotherExternalPort, ok := portNameMap[port.Name]; ok {
					externalPort = anotherExternalPort
				} else {
					externalPort = port.Port
				}
			}

			if externalPort <= 0 {
				r.Eventf(service, corev1.EventTypeNormal, EventReasonForward, "skip port %s due to %s annotation mapping it to %d", portName, AnnotationPortMap, externalPort)
				continue
			}

			description, ok := service.Annotations[AnnotationDescription]
			if !ok {
				description = fmt.Sprintf(
					"port-forward %s/%s port %s",
					service.ObjectMeta.Namespace, service.ObjectMeta.Name, portName,
				)
			}

			enabled, ok := service.Annotations[AnnotationEnabled]

			for _, ip := range ipAddresses {
				if err := r.AddPortMapping(ctx, &upnp.PortMapping{
					RemoteHost:     service.Annotations[AnnotationUPnPRemoteHost],
					ExternalPort:   externalPort,
					Protocol:       upnp.Protocol(port.Protocol),
					InternalPort:   port.Port,
					InternalClient: ip,
					Enabled:        !ok || isTruthy(enabled),
					Description:    description,
					LeaseDuration:  leaseDuration,
				}); err != nil {
					r.Eventf(service, corev1.EventTypeWarning, EventReasonForward, "%d to %s:%d for port %s failed with: %s", externalPort, ip, port.Port, portName, err.Error())
				} else {
					r.Eventf(service, corev1.EventTypeNormal, EventReasonForward, "%d to %s:%d for port %s", externalPort, ip, port.Port, portName)
				}
			}
		}
	} else {
		return ctrl.Result{RequeueAfter: time.Second * 9}, nil
	}

	if controllerutil.AddFinalizer(service, Finalizer) {
		if err := r.Client.Update(ctx, service); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func isTruthy(s string) bool {
	return xslice.Some([]string{"yes", "y", "1", "true"}, func(truthy string, _ int) bool {
		return strings.EqualFold(s, truthy)
	})
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("portfwd")

	return ctrl.NewControllerManagedBy(mgr).
		For(
			&corev1.Service{},
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				// Only reconcile if this Service has the port forward annotation or
				// finalizer. If it has the annotation, we need to port forward to it.
				// If it has the finalizer, then we may need to port forward to it
				// again or we may need to remove the finalizer.
				return isTruthy(obj.GetAnnotations()[AnnotationForward]) || controllerutil.ContainsFinalizer(obj, Finalizer)
			})),
		).
		Complete(r)
}
