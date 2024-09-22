package srcipmasqiptables

import (
	"context"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/frantjc/port-forward/internal/srcipmasq"
)

type SourceIPAddressMasqer struct {
	*iptables.IPTables

	mu sync.Mutex
}

func (m *SourceIPAddressMasqer) MasqSourceIPAddress(ctx context.Context, masq *srcipmasq.Masq) error {
	var (
		table    = "nat"
		chain    = "postrouting"
		ruleSpec = []string{
			"-s", masq.OriginalSource.String(),
			"-d", masq.Destination.String(),
			"-j", "SNAT",
			"--to-source", masq.NewSource.String(),
		}
	)

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.IPTables.Append(table, chain, ruleSpec...); err != nil {
		return err
	}

	if err := m.IPTables.DeleteIfExists(table, chain, ruleSpec...); err != nil {
		return err
	}

	return nil
}
