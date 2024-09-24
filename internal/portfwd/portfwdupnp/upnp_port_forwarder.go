package portfwdupnp

import (
	"context"

	"github.com/frantjc/port-forward/internal/portfwd"
	"github.com/frantjc/port-forward/internal/srcipmasq"
	"github.com/frantjc/port-forward/internal/upnp"
)

type PortForwarder struct {
	*upnp.Client
	srcipmasq.SourceIPAddressMasqer
}

func (p *PortForwarder) AddPortMapping(ctx context.Context, pm *portfwd.PortMapping) error {
	destination, err := p.GetServiceIPAddress(ctx)
	if err != nil {
		return err
	}

	restore, err := p.SourceIPAddressMasqer.MasqSourceIPAddress(ctx, &srcipmasq.Masq{
		OriginalSource: p.GetSourceIPAddress(ctx),
		Destination:    destination,
		NewSource:      pm.InternalClient,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = restore()
	}()

	return p.Client.AddPortMapping(ctx, pm)
}
