package srcipmasqnftables

import (
	"context"
	"fmt"
	"sync"

	"github.com/frantjc/port-forward/internal/srcipmasq"
	"github.com/google/nftables"
	"github.com/google/nftables/expr"
)

type SourceIPAddressMasqer struct {
	*nftables.Conn

	mu sync.Mutex
}

func (m *SourceIPAddressMasqer) MasqSourceIPAddress(ctx context.Context, masq *srcipmasq.Masq) error {
	var (
		family nftables.TableFamily
	)
	if masq.Destination.To4() != nil {
		family = nftables.TableFamilyIPv4
	} else if masq.Destination.To16() != nil {
		family = nftables.TableFamilyIPv6
	} else {
		return fmt.Errorf(`unable to determine family of destination IP address "%s"`, masq.Destination)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var (
		table = m.Conn.AddTable(&nftables.Table{
			Name:   "nat",
			Family: family,
		})
		chain = m.Conn.AddChain(&nftables.Chain{
			Name:     "postrouting",
			Table:    table,
			Hooknum:  nftables.ChainHookPostrouting,
			Priority: nftables.ChainPriorityNATSource,
			Type:     nftables.ChainTypeNAT,
		})
		_ = m.Conn.AddRule(&nftables.Rule{
			Table: table,
			Chain: chain,
			Exprs: []expr.Any{
				&expr.NAT{
					Type: expr.NATTypeSourceNAT,
				},
			},
		})
	)

	return m.Conn.Flush()
}
