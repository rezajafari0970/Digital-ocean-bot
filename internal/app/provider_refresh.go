package app

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// RefreshProviderSnapshots keeps capacity state fresh with one coordinated
// discovery per account. PostgreSQL advisory locks collapse concurrent workers.
func (c Container) RefreshProviderSnapshots(ctx context.Context, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 90 * time.Second
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT id::text FROM accounts WHERE enabled=true AND provider='digitalocean'`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		var fresh bool
		_ = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_snapshots WHERE account_id=$1 AND created_at > now()-($2 * interval '1 second'))`, id, int(maxAge/time.Second)).Scan(&fresh)
		if fresh {
			continue
		}
		var locked bool
		if err := c.DB.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "provider-refresh:"+id).Scan(&locked); err != nil || !locked {
			continue
		}
		func() {
			defer c.DB.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "provider-refresh:"+id)
			// Re-check after acquiring the lock: another coordinator may just have refreshed.
			_ = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_snapshots WHERE account_id=$1 AND created_at > now()-($2 * interval '1 second'))`, id, int(maxAge/time.Second)).Scan(&fresh)
			if fresh {
				return
			}
			rt, err := c.Runtime(ctx, id)
			if err != nil {
				return
			}
			d, err := rt.Provider.Discover(ctx)
			if rt.Gateway != nil {
				rt.Gateway.CloseIdleConnections()
			}
			if err != nil {
				log.Printf("provider refresh %s: %v", id, err)
				return
			}
			b, _ := json.Marshal(d)
			var oldLimit int
			_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE((data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&oldLimit)
			var snapshotID string
			if err = c.DB.QueryRowContext(ctx, `INSERT INTO provider_snapshots(id,account_id,provider,version,data) VALUES(gen_random_uuid(),$1,'digitalocean',1,$2) RETURNING id::text`, id, b).Scan(&snapshotID); err != nil {
				return
			}
			newLimit := d.Account.DropletLimit
			if oldLimit > 0 && newLimit > 0 && oldLimit != newLimit {
				_, _ = c.DB.ExecContext(ctx, `INSERT INTO account_capacity_events(account_id,old_limit,new_limit,delta,snapshot_id) VALUES($1,$2,$3,$4,$5)`, id, oldLimit, newLimit, newLimit-oldLimit, snapshotID)
				log.Printf("capacity change %s: %d -> %d (%+d)", id, oldLimit, newLimit, newLimit-oldLimit)
			}
			_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),updated_at=now() WHERE id=$1`, id, d.Account.UUID, d.Account.Email)
			c.ReconcileOwnedOrphans(ctx, id)
		}()
	}
}
