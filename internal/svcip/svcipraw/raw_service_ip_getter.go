package svcipraw

import (
	"net"

	xslices "github.com/frantjc/x/slices"
	corev1 "k8s.io/api/core/v1"
)

// ServiceIPAddressGetter implements svcip.ServiceIPAddressGetter
// by providing itself as the Service's IP addresses.
type ServiceIPAddressGetter []net.IP

// GetServiceIPAddresses implements svcip.ServiceIPAddressGetter.
func (g ServiceIPAddressGetter) GetServiceIPAddresses(*corev1.Service) []net.IP {
	return xslices.Map(g, func(ip net.IP, _ int) net.IP {
		return ip
	})
}
