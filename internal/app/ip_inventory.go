package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// ReadIPInventory preserves the existing account credential/proxy runtime.
// Callers must use ipam.InventoryStore.Refresh for durable admission and receipts.
func (c Container) ReadIPInventory(ctx context.Context, account string) (providers.IPObservation, error) {
	rt, e := c.Runtime(ctx, account)
	if rt.Gateway != nil {
		defer rt.Gateway.CloseIdleConnections()
	}
	if e != nil {
		return providers.IPObservation{}, e
	}
	reader, ok := rt.Driver.(providers.IPReader)
	if !ok {
		return providers.IPObservation{}, &providers.Error{Class: providers.ErrorUnavailable, Code: "IP_INVENTORY_UNSUPPORTED", Operation: "observe_ips"}
	}
	return reader.ObserveIPs(ctx)
}
