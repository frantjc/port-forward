package svcipdef

import (
	"net"

	xslice "github.com/frantjc/x/slice"
	corev1 "k8s.io/api/core/v1"
)

// ServiceIPAddressGetter implements svcip.ServiceIPAddressGetter
// by returning the given Service's IP addresses.
type ServiceIPAddressGetter struct{}

// GetServiceIPAddresses implements svcip.ServiceIPAddressGetter.
func (g *ServiceIPAddressGetter) GetServiceIPAddresses(svc *corev1.Service) []net.IP {
	return xslice.Map(svc.Status.LoadBalancer.Ingress, func(ingress corev1.LoadBalancerIngress, _ int) net.IP {
		return net.ParseIP(ingress.IP)
	})
}
