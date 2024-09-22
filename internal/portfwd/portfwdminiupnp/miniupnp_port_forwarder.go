package portfwdminiupnp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/frantjc/port-forward/internal/portfwd"
	"github.com/frantjc/port-forward/internal/srcipmasq"
	"github.com/frantjc/port-forward/internal/upnp"
)

func getScript(source, destination string, pm *portfwd.PortMapping) []byte {
	return []byte(fmt.Sprintf(`#!/bin/sh

iptables -t nat -A POSTROUTING --source %s --destination %s --jump SNAT --to-source %s

upnpc -a %s %d %d %s %d %s

iptables -t nat -D POSTROUTING --source %s --destination %s --jump SNAT --to-source %s
`,
		source, destination, pm.InternalClient,
		pm.InternalClient, pm.ExternalPort, pm.InternalPort, pm.Protocol, int(pm.LeaseDuration.Seconds()), pm.RemoteHost,
		source, destination, pm.InternalClient,
	))
}

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

	source := p.GetSourceIPAddress(ctx)

	script := getScript(source.String(), destination.String(), pm)

	tmp, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err = tmp.Write(script); err != nil {
		return err
	}

	if err = tmp.Chmod(0755); err != nil {
		return err
	}

	if err = tmp.Close(); err != nil {
		return err
	}

	return exec.CommandContext(ctx, tmp.Name()).Run()
}
