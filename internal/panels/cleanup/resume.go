package cleanup

import (
	"context"
	"errors"
)

var ErrResumeState = errors.New("saved cleanup is no longer resumable")

// Resume never creates a job. A stale page or a lost response cannot start
// another fleet deletion after the referenced job has already completed.
func (s Store) Resume(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout='4s'"); err != nil {
		return err
	}
	for _, lock := range []int64{628341902731, 628341902732} {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lock); err != nil {
			return err
		}
	}
	var state string
	if err = tx.QueryRowContext(ctx, "SELECT state FROM panel_cleanup_jobs WHERE id=$1 FOR UPDATE", id).Scan(&state); err != nil {
		return err
	}
	if state == "QUEUED" || state == "RUNNING" {
		return tx.Commit()
	}
	if state != "PAUSED" {
		return ErrResumeState
	}
	var frozen bool
	if err = tx.QueryRowContext(ctx, `SELECT NOT g.enabled AND (NOT m.enabled OR m.kill_switch) AND (NOT b.enabled OR b.kill_switch)
 AND NOT EXISTS(SELECT 1 FROM bulk_lifecycle_scopes WHERE enabled)
 FROM global_config_policies g CROSS JOIN client_mutation_execution_gate m CROSS JOIN bulk_client_execution_gate b WHERE g.policy_key='reality'`).Scan(&frozen); err != nil {
		return err
	}
	if !frozen {
		return ErrResumeState
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_targets SET state='PENDING',last_error='' WHERE job_id=$1 AND state='FAILED'", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_jobs SET state='QUEUED' WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit()
}
