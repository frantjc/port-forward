package main

import (
	"context"
	"fmt"
)

// +check
func (m *PortForwardDev) IsFmted(ctx context.Context) error {
	if empty, err := m.Fmt(ctx).IsEmpty(ctx); err != nil {
		return err
	} else if !empty {
		return fmt.Errorf("source is not formatted (run `dagger call fmt`)")
	}

	return nil
}

// +check
func (m *PortForwardDev) IsGenerated(ctx context.Context) error {
	if empty, err := m.Generate(ctx).IsEmpty(ctx); err != nil {
		return err
	} else if !empty {
		return fmt.Errorf("source is not generated (run `dagger call generate`)")
	}

	return nil
}

// +check
func (m *PortForwardDev) TestsPass(ctx context.Context) error {
	if _, err := m.Test(ctx); err != nil {
		return err
	}

	return nil
}
