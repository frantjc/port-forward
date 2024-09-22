package main

import (
	"context"
	"os"
	"os/exec"
)

func main() {
	var (
		ctx                  = context.Background()
		routerIPAddr         = "192.168.0.1"
		originalSourceIPAddr = "192.168.0.3"
		newSourceIPAddr      = "192.168.0.202"
		iptablesCommonArgs   = []string{
			"POSTROUTING",
			"--source", originalSourceIPAddr,
			"--destination", routerIPAddr,
			"--jump", "SNAT",
			"--to-source", newSourceIPAddr,
		}
		iptablesAppend = exec.CommandContext(ctx,
			"iptables",
			append([]string{"-t", "nat", "-A"}, iptablesCommonArgs...)...,
		)
		upnpcCommonArgs = []string{newSourceIPAddr, "80", "80", "TCP"}
		upnpcAdd        = exec.CommandContext(ctx, "upnpc", append([]string{"-a"}, upnpcCommonArgs...)...)
		upnpcDelete     = exec.CommandContext(ctx, "upnpc", append([]string{"-d"}, upnpcCommonArgs...)...)
		iptablesDelete  = exec.CommandContext(ctx,
			"iptables",
			append([]string{"-t", "nat", "-D"}, iptablesCommonArgs...)...,
		)
	)

	if err := run(iptablesAppend); err != nil {
		os.Exit(1)
	}

	if err := run(upnpcAdd); err != nil {
		os.Exit(1)
	}

	if err := run(upnpcDelete); err != nil {
		os.Exit(1)
	}

	if err := run(iptablesDelete); err != nil {
		os.Exit(1)
	}
}

func run(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
