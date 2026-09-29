package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"time"
)

var ErrCapacityUnavailable = errors.New("account droplet capacity unavailable")
var ErrCapacitySnapshotStale = errors.New("account capacity snapshot stale")

type CreateCapacity struct {
	LimitKnown       bool
	Limit            int
	ProviderDroplets int
	PendingCreates   int
	SnapshotAt       time.Time
}

func (x CreateCapacity) Available() int {
	if !x.LimitKnown {
		return int(^uint(0)>>1) - x.ProviderDroplets - x.PendingCreates
	}
	n := x.Limit - x.ProviderDroplets - x.PendingCreates
	if n < 0 {
		return 0
	}
	return n
}
func (c Container) RequireCreateCapacity(ctx context.Context, accountID string, maxAge time.Duration) (CreateCapacity, error) {
	var out CreateCapacity
	var enabled bool
	var runtimeStatus, providerState, providerError string
	if err := c.DB.QueryRowContext(ctx, `SELECT enabled,runtime_status,provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, accountID).Scan(&enabled, &runtimeStatus, &providerState, &providerError); err != nil || !enabled || runtimeStatus != "READY" || providerState != ProviderStateActive || providerError != "" {
		return out, ErrCapacityUnavailable
	}
	x, err := capacity.Read(ctx, c.DB, accountID, maxAge)
	if err != nil {
		return out, ErrCapacitySnapshotStale
	}
	out = CreateCapacity{LimitKnown: x.LimitKnown, Limit: x.Limit, ProviderDroplets: x.InUse, PendingCreates: x.Pending, SnapshotAt: x.ObservedAt}
	if (out.LimitKnown && out.Limit < 1) || out.Available() < 1 {
		return out, ErrCapacityUnavailable
	}
	return out, nil
}
