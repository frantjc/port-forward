package srcipmasq

import (
	"context"
	"net"
)

// Masq is the IP to masquerade as when targeting a specific destination.
type Masq struct {
	OriginalSource, Destination, NewSource net.IP
}

// SourceIPAddressMasqer masqs traffic to an IP address as an IP address.
type SourceIPAddressMasqer interface {
	MasqSourceIPAddress(context.Context, *Masq) (func() error, error)
}
