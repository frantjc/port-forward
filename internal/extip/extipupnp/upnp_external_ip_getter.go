package extipupnp

import (
	"github.com/frantjc/port-forward/internal/extip"
	"github.com/frantjc/port-forward/internal/upnp"
)

var _ extip.ExternalIPAddressGetter = &upnp.Client{}
