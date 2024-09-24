package extipraw

import (
	"context"
	"net"
)

// ExternalIPAddressGetter implements extip.ExternalIPAddressGetter
// by providing itself as the external IP address.
type ExternalIPAddressGetter net.IP

// GetExternalIPAddress implements extip.ExternalIPAddressGetter.
func (g ExternalIPAddressGetter) GetExternalIPAddress(ctx context.Context) (net.IP, error) {
	return net.IP(g), nil
}
