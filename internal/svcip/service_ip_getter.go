package svcip

import (
	"net"

	corev1 "k8s.io/api/core/v1"
)

// ServiceIPAddressGetter gets a Service's IP address.
type ServiceIPAddressGetter interface {
	GetServiceIPAddresses(*corev1.Service) []net.IP
}
