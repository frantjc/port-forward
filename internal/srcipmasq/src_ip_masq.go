package srcipmasq

import (
	"context"
	"net"
)

type Masq struct {
	OriginalSource, Destination, NewSource net.IP
}

type SourceIPAddressMasqer interface {
	MasqSourceIPAddress(context.Context, *Masq) error
}
