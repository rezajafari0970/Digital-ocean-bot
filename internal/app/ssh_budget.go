package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

// Only the initial, read-only SSH probe has this wall-clock budget. An
// interrupted installation or an unknown provider mutation must be reconciled,
// never treated as a disposable boot failure.
func (h RecoveryHandler) expireInitialSSH(ctx context.Context, d workflow.Deployment) (bool, error) {
	if d.CurrentStep != "provision" || d.DropletID == "" {
		return false, nil
	}
	release, err := (workflow.PostgresRunLease{DB: h.Container.DB}).Acquire(ctx, d.ID)
	if errors.Is(err, workflow.ErrDeploymentBusy) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer release()
	tx, err := h.Container.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var runID, last string
	err = tx.QueryRowContext(ctx, `SELECT pr.id::text,COALESCE(NULLIF(ps.last_error,''),NULLIF(pr.last_error,''),'initial SSH unavailable')
 FROM deployments d JOIN droplets v ON v.id=d.droplet_id AND v.account_id=d.account_id
 JOIN accounts a ON a.id=d.account_id
 JOIN provision_runs pr ON pr.droplet_id=v.id AND pr.account_id=d.account_id
 JOIN provision_step_attempts ps ON ps.run_id=pr.id AND ps.step='ssh'
 WHERE d.id=$1 AND d.account_id=$2 AND d.current_step='provision'
 AND d.state NOT IN ('READY','PANEL_COMPLETE','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')
 AND v.state='PROVISIONING' AND v.provider_resource_id<>'' AND a.deletion_requested_at IS NULL
 AND pr.current_step='ssh' AND pr.state IN ('PENDING','WAITING_SSH')
 AND pr.created_at<now()-interval '15 minutes' AND ps.attempts>=3
 AND ps.last_finished_at>=ps.last_started_at AND ps.last_error IS NOT NULL
 FOR UPDATE OF d,v,pr,ps`, d.ID, d.AccountID).Scan(&runID, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	msg := "INITIAL_SSH_BUDGET_EXHAUSTED: " + last
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE deployments SET state='FAILED',current_step='done',last_error=$2,lock_version=lock_version+1,updated_at=now() WHERE id=$1`, []any{d.ID, msg}},
		{`UPDATE provision_runs SET state='FAILED',last_error=$2,next_retry_at=NULL,updated_at=now() WHERE id=$1`, []any{runID, msg}},
		{`UPDATE provision_step_attempts SET terminal=true,next_retry_at=NULL WHERE run_id=$1 AND step='ssh'`, []any{runID}},
		{`UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1`, []any{d.DropletID}},
		{`INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')`, []any{d.AccountID, d.DropletID}},
		{`INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,'provision','FAILED',$2)`, []any{d.ID, msg}},
	} {
		if _, err = tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
