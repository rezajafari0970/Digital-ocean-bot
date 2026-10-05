package residentialperf

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Publication includes currently tracked enabled panels, even while their
// network/runtime verification is pending. The existing routing worker/gates
// decide when they can actually apply. Future panels are admitted in bounded batches.
const publicationFrom = ` FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
 WHERE p.enabled AND d.state NOT IN('DELETED','DELETING','RETIRING')
 AND (d.expires_at IS NULL OR d.expires_at>now())
 AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL `

func publishable(ctx context.Context, tx *sql.Tx, panel string) error {
	var ok bool
	e := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1"+publicationFrom+" AND p.id=$1)", panel).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return conflict("selected server was removed, disabled or expired; refresh selection")
	}
	return nil
}
func enrollFleet(ctx context.Context, tx *sql.Tx, experiment string, c *Config) error {
	rows, e := tx.QueryContext(ctx, "SELECT p.id::text"+publicationFrom+`
 AND NOT EXISTS(SELECT 1 FROM residential_performance_targets t WHERE t.experiment_id=$1 AND t.panel_id=p.id)
 ORDER BY p.id LIMIT 64`, experiment)
	if e != nil {
		return e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = attach(ctx, tx, experiment, id, c); e != nil {
			return e
		}
	}
	return nil
}

// Caller holds the same transaction/advisory lock used by rollback. There can
// be no admission after rollback has closed this policy.
func enrollPublished(ctx context.Context, tx *sql.Tx) error {
	var id string
	var spec []byte
	e := tx.QueryRowContext(ctx, "SELECT id::text,spec FROM residential_performance_experiments WHERE state='KEPT' AND duration_mode='permanent' AND publish_scope='fleet' FOR UPDATE").Scan(&id, &spec)
	if e == sql.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	var c Config
	if e = json.Unmarshal(spec, &c); e != nil {
		return e
	}
	return enrollFleet(ctx, tx, id, &c)
}
