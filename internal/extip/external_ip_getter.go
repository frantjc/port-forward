package extip

import (
	"context"
	"net"
)

// ExternalIPAddressGetter gets the external IP address.
type ExternalIPAddressGetter interface {
	GetExternalIPAddress(context.Context) (net.IP, error)
}
