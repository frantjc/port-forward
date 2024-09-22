package portfwdminiupnp

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/frantjc/port-forward/internal/portfwd"
	"github.com/frantjc/port-forward/internal/srcipmasq"
	"github.com/frantjc/port-forward/internal/upnp"
)

type PortForwarder struct {
	*upnp.Client
	srcipmasq.SourceIPAddressMasqer
}

func NewPortForwarder(client *upnp.Client, masqer srcipmasq.SourceIPAddressMasqer) portfwd.PortForwarder {
	return &PortForwarder{client, masqer}
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

	args := []string{
		"-a",
		pm.InternalClient.String(),
		fmt.Sprint(pm.InternalPort), fmt.Sprint(pm.ExternalPort),
		string(pm.Protocol),
	}
	if pm.LeaseDuration != 0 {
		args = append(args, fmt.Sprint(int(pm.LeaseDuration.Seconds())))
	}
	if pm.RemoteHost != "" {
		args = append(args, pm.RemoteHost)
	}

	return exec.CommandContext(ctx,
		"upnpc", args...,
	).Run()
}
