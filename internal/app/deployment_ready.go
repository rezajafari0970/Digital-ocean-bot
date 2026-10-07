package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

type deploymentReadyFinalizer struct {
	DB       workflow.DBTX
	Lifetime time.Duration
}

func (f deploymentReadyFinalizer) MarkReady(ctx context.Context, d workflow.Deployment) error {
	if db, ok := f.DB.(*sql.DB); ok && db != nil {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = f.MarkReadyIn(ctx, tx, d); err != nil {
			return err
		}
		return tx.Commit()
	}
	if f.DB == nil || d.DropletID == "" {
		return workflow.ErrRuntimeConfig
	}
	seconds := int64(f.Lifetime / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	res, err := f.DB.ExecContext(ctx, `UPDATE droplets SET state='READY',ready_at=COALESCE(ready_at,now()),expires_at=COALESCE(expires_at,now()+($3 * interval '1 second')),updated_at=now() WHERE id=$1 AND account_id=$2 AND state IN ('PROVISIONING','READY')`, d.DropletID, d.AccountID, seconds)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return workflow.ErrDeploymentVersionConflict
	}
	var runID string
	err = f.DB.QueryRowContext(ctx, `UPDATE provision_runs SET state='COMPLETED',current_step='done',last_error=NULL,next_retry_at=NULL,updated_at=now()
 WHERE account_id=$1 AND droplet_id=$2 AND state<>'FAILED' RETURNING id::text`, d.AccountID, d.DropletID).Scan(&runID)
	if err != nil {
		return err
	}
	_, err = f.DB.ExecContext(ctx, `UPDATE provision_step_attempts SET next_retry_at=NULL WHERE run_id=$1`, runID)
	return err
}

type deploymentFailureFinalizer struct{ DB workflow.DBTX }

func (f deploymentFailureFinalizer) MarkFailed(ctx context.Context, d workflow.Deployment) error {
	if db, ok := f.DB.(*sql.DB); ok && db != nil && d.DropletID != "" {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = (deploymentFailureFinalizer{DB: tx}).MarkFailed(ctx, d); err != nil {
			return err
		}
		return tx.Commit()
	}
	if d.DropletID == "" {
		return nil // A create failure before a resource exists has nothing to retire.
	}
	if f.DB == nil {
		return workflow.ErrRuntimeConfig
	}
	var resourceState string
	if err := f.DB.QueryRowContext(ctx, `SELECT state FROM droplets WHERE id=$1 AND account_id=$2 FOR UPDATE`, d.DropletID, d.AccountID).Scan(&resourceState); err != nil {
		return err
	}
	switch resourceState {
	case "PROVISIONING":
		res, err := f.DB.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND account_id=$2 AND state='PROVISIONING'`, d.DropletID, d.AccountID)
		if err = requireRecoveryOperationUpdate(res, err); err != nil {
			return err
		}
		if _, err = f.DB.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')`, d.AccountID, d.DropletID); err != nil {
			return err
		}
	case "RETIRING", "DELETING", "DELETED":
		// A committed prior finalization may already have advanced retirement.
	default:
		return workflow.ErrDeploymentVersionConflict
	}
	var runID, runState string
	if err := f.DB.QueryRowContext(ctx, `SELECT id::text,state FROM provision_runs WHERE account_id=$1 AND droplet_id=$2 FOR UPDATE`, d.AccountID, d.DropletID).Scan(&runID, &runState); err != nil {
		return err
	}
	if runState == "COMPLETED" {
		return workflow.ErrDeploymentVersionConflict
	}
	res, err := f.DB.ExecContext(ctx, `UPDATE provision_runs SET state='FAILED',last_error=COALESCE(NULLIF(last_error,''),'deployment terminal failure'),next_retry_at=NULL,updated_at=now() WHERE id=$1 AND state=$2`, runID, runState)
	if err = requireRecoveryOperationUpdate(res, err); err != nil {
		return err
	}
	_, err = f.DB.ExecContext(ctx, `UPDATE provision_step_attempts SET next_retry_at=NULL WHERE run_id=$1`, runID)
	return err
}

func (f deploymentReadyFinalizer) MarkReadyIn(ctx context.Context, tx workflow.DBTX, d workflow.Deployment) error {
	f.DB = tx
	return f.MarkReady(ctx, d)
}
func (f deploymentFailureFinalizer) MarkFailedIn(ctx context.Context, tx workflow.DBTX, d workflow.Deployment) error {
	f.DB = tx
	return f.MarkFailed(ctx, d)
}
