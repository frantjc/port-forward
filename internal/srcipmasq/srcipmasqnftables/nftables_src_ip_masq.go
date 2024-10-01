package srcipmasqnftables

import (
	"context"
	"fmt"

	"github.com/frantjc/port-forward/internal/srcipmasq"
	"github.com/google/nftables"
	"github.com/google/nftables/expr"
)

// SourceIPAddressMasqer implements srcipmasq.SourceIPAddressMasqer
// using nftables.
type SourceIPAddressMasqer struct {
	*nftables.Conn
}

// MasqSourceIPAddress implements srcipmasq.SourceIPAddressMasqer.
func (m *SourceIPAddressMasqer) MasqSourceIPAddress(ctx context.Context, masq *srcipmasq.Masq) (func() error, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (m *SourceIPAddressMasqer) masqSourceIPAddress(ctx context.Context, masq *srcipmasq.Masq) (func() error, error) {
	var (
		family nftables.TableFamily
	)
	if masq.Destination.To4() != nil {
		family = nftables.TableFamilyIPv4
	} else if masq.Destination.To16() != nil {
		family = nftables.TableFamilyIPv6
	} else {
		return nil, fmt.Errorf("unable to determine family of destination IP address %s", masq.Destination)
	}

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
		rule = m.Conn.AddRule(&nftables.Rule{
			Table: table,
			Chain: chain,
			Exprs: []expr.Any{
				&expr.NAT{
					Type: expr.NATTypeSourceNAT,
				},
			},
		})
	)

	return func() error {
		return m.Conn.DelRule(rule)
	}, m.Conn.Flush()
}
