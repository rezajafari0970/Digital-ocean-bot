package capacity

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrSnapshotMissing = errors.New("provider capacity snapshot missing")
var ErrSnapshotStale = errors.New("provider capacity snapshot stale")

type Querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Snapshot struct {
	Limit, InUse, Pending int
	ObservedAt            time.Time
}

func (x Snapshot) Available() int {
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
	err := q.QueryRowContext(ctx, `SELECT COALESCE((ps.data->'Limits'->>'DropletLimit')::int,0),jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb)),(SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND COALESCE(resource_id,'')='' AND (state IN ('planned','running','verifying') OR (state='unknown' AND updated_at > now()-interval '2 minutes'))),ps.created_at FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1`, accountID).Scan(&x.Limit, &x.InUse, &x.Pending, &x.ObservedAt)
	if err != nil {
		return x, ErrSnapshotMissing
	}
	if time.Since(x.ObservedAt) > maxAge {
		return x, ErrSnapshotStale
	}
	return x, nil
}
