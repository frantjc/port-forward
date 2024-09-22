package extipenv

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/frantjc/port-forward/internal/extip"
)

// NewExternalIPAddressGetter returns an ExternalIPAddressGetter
// that gets the external IP address from the value of the given
// environment variable.
func NewExternalIPAddressGetter(envVar string) extip.ExternalIPAddressGetter {
	return ExternalIPAddressGetter(envVar)
}

// ExternalIPAddressGetter implements extip.ExternalIPAddressGetter
// by getting the external IP address from the value of an envionment
// variable.
type ExternalIPAddressGetter string

// GetExternalIPAddress gets the external IP address from the value of an
// environment variable.
func (g ExternalIPAddressGetter) GetExternalIPAddress(ctx context.Context) (net.IP, error) {
	ips := os.Getenv(string(g))

	ip := net.ParseIP(ips)
	if ip == nil {
		return nil, fmt.Errorf("unable to parse ip: %s", ips)
	}

	return ip, nil
}
