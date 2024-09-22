package controller

import (
	"context"
	"strings"

	"github.com/frantjc/port-forward/internal/extip"
	xslice "github.com/frantjc/x/slice"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UPnPIngressReconciler struct {
	extip.ExternalIPAddressGetter
	client.Client
	record.EventRecorder
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingress,verbs=get;list;watch;update

func (r *UPnPIngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		ingress = &networkingv1.Ingress{}
	)

	if err := r.Client.Get(ctx, req.NamespacedName, ingress); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	enabled, ok := ingress.Annotations[UPnPEnabledAnnotation]
	if !ok {
		return ctrl.Result{}, nil
	}

	if !xslice.Some([]string{"yes", "y", "1", "true"}, func(truthy string, _ int) bool {
		return strings.EqualFold(enabled, truthy)
	}) {
		r.Eventf(ingress, corev1.EventTypeNormal, "InvalidAnnotation", `redundant falsy value "%s" in "%s" annotation`, enabled, UPnPEnabledAnnotation)
		return ctrl.Result{}, nil
	}

	externalIPAddress, err := r.GetExternalIPAddress(ctx)
	if err != nil {
		r.Eventf(ingress, corev1.EventTypeWarning, "FailedGetExternalIPAddress", "get external IP address failed with: %s", err.Error())
		return ctrl.Result{}, nil
	}

	ingress.Annotations["external-dns.alpha.kubernetes.io/target"] = externalIPAddress.String()

	if err = r.Client.Update(ctx, ingress); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *UPnPIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("homelab")

	return ctrl.NewControllerManagedBy(mgr).
		Named("ingress-upnp").
		For(&networkingv1.Ingress{}).
		Complete(r)
}
