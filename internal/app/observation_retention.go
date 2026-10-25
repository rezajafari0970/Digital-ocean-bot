package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"log"
	"time"
)

// Successful observations are replaceable telemetry, not mutation journals.
// Retain a week, the latest proof, all failures, and capacity-event evidence.
// Every batch is bounded; normal PostgreSQL autovacuum reuses freed space.
func (c Container) PruneObservations(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var total int64
	// Ordered single-row probes avoid a large semi-join when a burst has not
	// yet been analyzed. Lock only the doomed row, never its retained witness.
	queries := []string{
		`WITH doomed AS (SELECT s.id FROM panel_inventory_syncs s WHERE s.state='COMPLETED' AND s.finished_at<now()-interval '7 days'
 AND s.finished_at < (SELECT newer.finished_at FROM panel_inventory_syncs newer WHERE newer.panel_id=s.panel_id AND newer.state='COMPLETED' AND newer.finished_at IS NOT NULL ORDER BY newer.finished_at DESC LIMIT 1)
 ORDER BY s.finished_at LIMIT 1000 FOR UPDATE OF s SKIP LOCKED)
 DELETE FROM panel_inventory_syncs s USING doomed d WHERE s.id=d.id`,
		`WITH doomed AS (SELECT s.id FROM provider_snapshots s WHERE s.canonical->'Account'->>'Status'='active' AND s.created_at<now()-interval '7 days'
 AND NOT EXISTS(SELECT 1 FROM account_capacity_events e WHERE e.snapshot_id=s.id)
 AND s.created_at < (SELECT newer.created_at FROM provider_snapshots newer WHERE newer.account_id=s.account_id AND newer.canonical IS NOT NULL ORDER BY newer.created_at DESC LIMIT 1)
 ORDER BY s.created_at LIMIT 1000 FOR UPDATE OF s SKIP LOCKED)
 DELETE FROM provider_snapshots s USING doomed d WHERE s.id=d.id`,
	}
	// Keep each transaction small, but allow up to ten batches per table to
	// catch up after downtime or bursts. The existing ten-second deadline bounds
	// the whole round; SKIP LOCKED lets independent controllers share the work.
	pending := make([]bool, len(queries))
	for i := range pending {
		pending[i] = true
	}
	for batch := 0; batch < 10; batch++ {
		more := false
		for i, query := range queries {
			if !pending[i] {
				continue
			}
			if err := ctx.Err(); err != nil {
				return total, err
			}
			res, err := c.DB.ExecContext(ctx, query)
			if err != nil {
				return total, err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return total, err
			}
			total += n
			pending[i] = n == 1000
			more = more || pending[i]
		}
		if !more {
			break
		}
	}

	return total, nil
}
func (c Container) RunObservationRetention(ctx context.Context) {
	timer := time.NewTicker(5 * time.Minute)
	defer timer.Stop()
	for {
		supervision.Pulse(ctx)
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			n, err := c.PruneObservations(ctx)
			if err != nil && ctx.Err() == nil {
				log.Printf("observation retention: %v", err)
			} else if n > 0 {
				log.Printf("observation retention removed=%d", n)
			}
		}
	}
}
