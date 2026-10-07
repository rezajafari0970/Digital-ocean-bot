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
	for _, query := range []string{
		`WITH doomed AS (SELECT s.id FROM panel_inventory_syncs s WHERE s.state='COMPLETED' AND s.finished_at<now()-interval '7 days'
 AND EXISTS(SELECT 1 FROM panel_inventory_syncs newer WHERE newer.panel_id=s.panel_id AND newer.state='COMPLETED' AND newer.finished_at>s.finished_at)
 ORDER BY s.finished_at LIMIT 1000 FOR UPDATE SKIP LOCKED)
 DELETE FROM panel_inventory_syncs s USING doomed d WHERE s.id=d.id`,
		`WITH doomed AS (SELECT s.id FROM provider_snapshots s WHERE s.canonical->'Account'->>'Status'='active' AND s.created_at<now()-interval '7 days'
 AND NOT EXISTS(SELECT 1 FROM account_capacity_events e WHERE e.snapshot_id=s.id)
 AND EXISTS(SELECT 1 FROM provider_snapshots newer WHERE newer.account_id=s.account_id AND newer.canonical IS NOT NULL AND newer.created_at>s.created_at)
 ORDER BY s.created_at LIMIT 1000 FOR UPDATE SKIP LOCKED)
 DELETE FROM provider_snapshots s USING doomed d WHERE s.id=d.id`,
	} {
		res, err := c.DB.ExecContext(ctx, query)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
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
