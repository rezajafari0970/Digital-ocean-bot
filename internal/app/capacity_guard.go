package app

import (
	"context"
	"errors"
	"time"
)

var ErrCapacityUnavailable = errors.New("account droplet capacity unavailable")
var ErrCapacitySnapshotStale = errors.New("account capacity snapshot stale")

type CreateCapacity struct {
	Limit            int
	ProviderDroplets int
	PendingCreates   int
	SnapshotAt       time.Time
}

func (x CreateCapacity) Available() int {
	n := x.Limit - x.ProviderDroplets - x.PendingCreates
	if n < 0 {
		return 0
	}
	return n
}
func (c Container) RequireCreateCapacity(ctx context.Context, accountID string, maxAge time.Duration) (CreateCapacity, error) {
	if maxAge <= 0 {
		maxAge = 2 * time.Minute
	}
	var x CreateCapacity
	var enabled bool
	var runtimeStatus, providerState, providerError string
	if err := c.DB.QueryRowContext(ctx, `SELECT enabled,runtime_status,provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, accountID).Scan(&enabled, &runtimeStatus, &providerState, &providerError); err != nil || !enabled || runtimeStatus != "READY" || providerState != ProviderStateActive || providerError != "" {
		return x, ErrCapacityUnavailable
	}
	err := c.DB.QueryRowContext(ctx, `SELECT
 COALESCE((ps.data->'Limits'->>'DropletLimit')::int,0),
 jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb)),
 (SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state IN ('planned','running','verifying','unknown') AND COALESCE(resource_id,'')=''),
 ps.created_at
 FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1`, accountID).Scan(&x.Limit, &x.ProviderDroplets, &x.PendingCreates, &x.SnapshotAt)
	if err != nil {
		return x, ErrCapacitySnapshotStale
	}
	if time.Since(x.SnapshotAt) > maxAge {
		return x, ErrCapacitySnapshotStale
	}
	if x.Limit < 1 || x.Available() < 1 {
		return x, ErrCapacityUnavailable
	}
	return x, nil
}
