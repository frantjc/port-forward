package portfwdupnp

import (
	"context"
	"fmt"

	"github.com/frantjc/port-forward/internal/portfwd"
	"github.com/frantjc/port-forward/internal/upnp"
	xslice "github.com/frantjc/x/slice"
	"github.com/google/nftables"
	"github.com/google/nftables/expr"
)

type PortForwarder struct {
	*upnp.Client
}

func NewPortForwarder(client *upnp.Client) portfwd.PortForwarder {
	return &PortForwarder{client}
}

func (p *PortForwarder) AddPortMapping(ctx context.Context, pm *portfwd.PortMapping) error {
	conn, err := nftables.New()
	if err != nil {
		return err
	}

	ip, err := p.GetServiceIPAddress(ctx)
	if err != nil {
		return err
	}

	var (
		family nftables.TableFamily
	)
	if ip.To4() != nil {
		family = nftables.TableFamilyIPv4
	} else if ip.To16() != nil {
		family = nftables.TableFamilyIPv6
	} else {
		return fmt.Errorf(
			`unable to determine family of UPnP service location IP address "%s"`,
			xslice.Join(ip, ", "),
		)
	}

	var (
		table = conn.CreateTable(&nftables.Table{
			Name:   "port-forward-controller",
			Family: family,
		})
		chain = conn.AddChain(&nftables.Chain{
			Name:    "port-forward-controller",
			Table:   table,
			Type:    nftables.ChainTypeNAT,
			Hooknum: nftables.ChainHookPostrouting,
		})
		_ = conn.AddRule(&nftables.Rule{
			Table: table,
			Chain: chain,
			Exprs: []expr.Any{},
		})
	)

	return p.Client.AddPortMapping(ctx, pm)
}
