package srcipmasqiptables

import (
	"context"

	"github.com/coreos/go-iptables/iptables"
	"github.com/frantjc/port-forward/internal/srcipmasq"
)

// SourceIPAddressMasqer implements srcipmasq.SourceIPAddressMasqer
// using `iptables`.
type SourceIPAddressMasqer struct {
	*iptables.IPTables
}

// MasqSourceIPAddress implements srcipmasq.SourceIPAddressMasqer.
func (m *SourceIPAddressMasqer) MasqSourceIPAddress(ctx context.Context, masq *srcipmasq.Masq) (func() error, error) {
	var (
		table    = "nat"
		chain    = "POSTROUTING"
		ruleSpec = []string{
			"--source", masq.OriginalSource.String(),
			"--destination", masq.Destination.String(),
			"--jump", "SNAT",
			"--to-source", masq.NewSource.String(),
		}
	)

	if err := m.IPTables.Append(table, chain, ruleSpec...); err != nil {
		return nil, err
	}
	return func() error {
		return m.IPTables.DeleteIfExists(table, chain, ruleSpec...)
	}, nil
}
