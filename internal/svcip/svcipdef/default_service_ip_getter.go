package svcipdef

import (
	"net"

	xslices "github.com/frantjc/x/slices"
	corev1 "k8s.io/api/core/v1"
)

// ServiceIPAddressGetter implements svcip.ServiceIPAddressGetter
// by returning the given Service's IP addresses.
type ServiceIPAddressGetter struct{}

// GetServiceIPAddresses implements svcip.ServiceIPAddressGetter.
func (g *ServiceIPAddressGetter) GetServiceIPAddresses(svc *corev1.Service) []net.IP {
	return xslices.Map(svc.Status.LoadBalancer.Ingress, func(ingress corev1.LoadBalancerIngress, _ int) net.IP {
		return net.ParseIP(ingress.IP)
	})
}
