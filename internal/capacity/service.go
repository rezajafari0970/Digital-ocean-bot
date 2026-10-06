package capacity

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

var ErrSnapshotMissing = errors.New("provider capacity snapshot missing")
var ErrSnapshotStale = errors.New("provider capacity snapshot stale")

type Querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Snapshot struct {
	Limit, InUse, Pending int
	LimitKnown            bool
	ObservedAt            time.Time
}

func (x Snapshot) Available() int {
	if !x.LimitKnown {
		return math.MaxInt32 - x.InUse - x.Pending
	}
	n := x.Limit - x.InUse - x.Pending
	if n < 0 {
		return 0
	}
	return n
}
func Read(ctx context.Context, q Querier, accountID string, maxAge time.Duration) (Snapshot, error) {
	if maxAge <= 0 {
		maxAge = 2 * time.Minute
	}
	var x Snapshot
	var accountStatus, providerName string
	block, blockErr := ReadCreateBlock(ctx, q, accountID)
	if blockErr != nil {
		return x, blockErr
	}
	if block != nil {
		return x, ErrCreateBlocked
	}
	err := q.QueryRowContext(ctx, `SELECT COALESCE((ps.canonical->'Capacity'->>'ComputeLimit')::int,(ps.data->'Limits'->>'DropletLimit')::int,0),COALESCE((ps.canonical->'Capacity'->>'LimitKnown')::boolean,CASE WHEN COALESCE((ps.canonical->'Capacity'->>'ComputeLimit')::int,(ps.data->'Limits'->>'DropletLimit')::int,0)>0 THEN true ELSE false END),COALESCE((ps.canonical->'Capacity'->>'ComputeInUse')::int,jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb)),0),(SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND COALESCE(resource_id,'')='' AND state IN ('planned','running','verifying','unknown')),ps.created_at,COALESCE(ps.canonical->'Account'->>'Status',''),ps.provider FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1`, accountID).Scan(&x.Limit, &x.LimitKnown, &x.InUse, &x.Pending, &x.ObservedAt, &accountStatus, &providerName)
	if err != nil {
		return x, ErrSnapshotMissing
	}
	if time.Since(x.ObservedAt) > maxAge {
		return x, ErrSnapshotStale
	}
	if providerName == "upcloud" && !x.LimitKnown {
		return x, ErrSnapshotMissing
	}
	if accountStatus == "trial_restricted" {
		allowed, err := AccountStatusAllowsCreate(ctx, q, accountID, accountStatus)
		if err != nil {
			return x, err
		}
		if !allowed {
			return x, ErrCreateBlocked
		}
	}
	return x, nil
}
