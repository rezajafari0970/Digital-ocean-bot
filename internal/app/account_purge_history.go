package app

import (
	"context"
	"database/sql"
	"time"
)

// Only disposable observations are pruned before the final atomic purge. Cloud
// ownership, operations, credentials, recovery checkpoints and configuration
// survive every partial pass. Commit bounded chunks so cancellation cannot force
// hundreds of thousands of historical rows to be removed from scratch again.
func pruneSavedAccountHistory(ctx context.Context, db *sql.DB, id, state string, remaining int, confirmation AccountPurgeConfirmation) (bool, int64, error) {
	deadline := time.Now().Add(8 * time.Second)
	var total int64
	for {
		tx, e := openAccountPurge(ctx, db, id, state, remaining, confirmation)
		if e != nil || tx == nil {
			return false, total, e
		}
		queries := []string{
			`DELETE FROM account_capacity_events WHERE ctid IN(SELECT ctid FROM account_capacity_events WHERE account_id=$1 LIMIT 1000)`,
			`DELETE FROM provider_snapshots WHERE id IN(SELECT id FROM provider_snapshots WHERE account_id=$1 LIMIT 500)`,
			`DELETE FROM panel_inventory_syncs WHERE ctid IN(SELECT h.ctid FROM panel_inventory_syncs h JOIN panel_instances p ON p.id=h.panel_id WHERE p.account_id=$1 LIMIT 10000)`,
			`DELETE FROM provision_events WHERE ctid IN(SELECT h.ctid FROM provision_events h JOIN provision_runs r ON r.id=h.run_id JOIN droplets d ON d.id=r.droplet_id WHERE d.account_id=$1 LIMIT 5000)`,
			`DELETE FROM provision_step_attempts WHERE ctid IN(SELECT h.ctid FROM provision_step_attempts h JOIN provision_runs r ON r.id=h.run_id JOIN droplets d ON d.id=r.droplet_id WHERE d.account_id=$1 LIMIT 5000)`,
		}
		var changed int64
		for _, q := range queries {
			r, err := tx.ExecContext(ctx, q, id)
			if err != nil {
				tx.Rollback()
				return false, total, err
			}
			n, err := r.RowsAffected()
			if err != nil {
				tx.Rollback()
				return false, total, err
			}
			changed += n
		}
		if e = tx.Commit(); e != nil {
			return false, total, e
		}
		total += changed
		if changed == 0 {
			return true, total, nil
		}
		if time.Now().After(deadline) {
			return false, total, nil
		}
	}
}
