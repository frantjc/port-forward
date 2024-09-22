package portfwd

import (
	"context"

	"github.com/frantjc/port-forward/internal/upnp"
)

type (
	PortMapping = upnp.PortMapping
)

// PortForwarder forwards the given port.
type PortForwarder interface {
	AddPortMapping(context.Context, *PortMapping) error
}
