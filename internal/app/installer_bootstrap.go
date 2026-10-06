package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

const bootstrapRepairPending = "BOOTSTRAP_REPAIR_PENDING"
const bootstrapRepairReboot = "BOOTSTRAP_REPAIR_REBOOT"

var errInstallerBootstrapEvidence = errors.New("installer bootstrap completion evidence missing")

// reconcileInstallerBootstrap is called under the deployment run lease. It
// authorizes activation only from durable prerequisite completions, not a label
// written by recovery. Repair changes checkpoints atomically; attempt/backoff
// history and the immutable plan are never reset.
func (c Container) reconcileInstallerBootstrap(ctx context.Context, d workflow.Deployment, plan provisioning.Plan) (bool, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var state string
	var version int64
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT d.state,d.lock_version,
 a.deletion_requested_at IS NULL AND r.state='PROVISIONING'
 FROM deployments d JOIN accounts a ON a.id=d.account_id
 JOIN droplets r ON r.id=d.droplet_id AND r.account_id=d.account_id
 WHERE d.id=$1 AND d.account_id=$2 FOR UPDATE OF d`, d.ID, d.AccountID).Scan(&state, &version, &eligible)
	if err != nil {
		return false, err
	}
	if state != string(workflow.WaitingInstaller) || version != d.LockVersion || !eligible {
		return false, workflow.ErrDeploymentVersionConflict
	}
	var runID, runState, current string
	err = tx.QueryRowContext(ctx, `SELECT id::text,state,current_step FROM provision_runs WHERE account_id=$1 AND droplet_id=$2 FOR UPDATE`, d.AccountID, d.DropletID).Scan(&runID, &runState, &current)
	if err != nil {
		return false, err
	}
	if runState == string(provisioning.Failed) {
		return false, provisioning.ErrStepTerminal
	}
	before, placeholder, err := provisioning.InstallerPrerequisites(plan, true)
	if err != nil {
		return false, err
	}
	completed := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT step,last_started_at IS NOT NULL AND last_finished_at IS NOT NULL AND last_finished_at>=last_started_at AND last_error IS NULL AND NOT terminal FROM provision_step_attempts WHERE run_id=$1`, runID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var step string
		var done bool
		if err = rows.Scan(&step, &done); err != nil {
			rows.Close()
			return false, err
		}
		completed[step] = done
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	first := ""
	for _, step := range before {
		if !completed[step] {
			first = step
			break
		}
	}
	if first == "" && current == placeholder && runState == string(provisioning.WaitingInstaller) {
		return true, tx.Commit()
	}
	// Once an installer has started, missing evidence needs diagnosis, not a
	// bootstrap replay over potentially installed services.
	var installed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM installer_runs WHERE deployment_id=$1)`, d.ID).Scan(&installed); err != nil {
		return false, err
	}
	if installed {
		return false, errInstallerBootstrapEvidence
	}
	if first == "" {
		first = placeholder
	}
	_, err = tx.ExecContext(ctx, `UPDATE provision_runs SET state='PENDING',current_step=$2,last_error='BOOTSTRAP_RESUME_REQUIRED',updated_at=now() WHERE id=$1`, runID, first)
	if err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE deployments SET state='PROVISIONING',current_step='provision',last_error='BOOTSTRAP_RESUME_REQUIRED',lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$3 AND state='WAITING_INSTALLER'`, d.ID, d.AccountID, d.LockVersion)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, workflow.ErrDeploymentVersionConflict
	}
	// An old premature reboot cannot activate a guard that had not run. Permit
	// one correction only after that exact guard later durably completes.
	for _, step := range before {
		if step == "bootstrap-kernel-memory-guard" && !completed[step] {
			_, err = tx.ExecContext(ctx, `UPDATE installer_reboot_remediations SET last_error=$2||';previous_scheduled_at='||scheduled_at::text WHERE deployment_id=$1 AND generation=(SELECT installer_generation FROM deployments WHERE id=$1) AND state='SCHEDULED' AND last_error NOT LIKE 'BOOTSTRAP_REPAIR_%'`, d.ID, bootstrapRepairPending)
			if err != nil {
				return false, err
			}
		}
	}
	if err = (workflow.SQLStore{DB: tx}).Event(ctx, d.ID, "provision", workflow.Provisioning, "BOOTSTRAP_CHECKPOINT_REPAIRED:"+first); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

// claimBootstrapRepairReboot preserves the old attempt as an event and consumes
// the one repair marker atomically. It never rearms an ordinary reboot timeout.
func (c Container) claimBootstrapRepairReboot(ctx context.Context, d workflow.Deployment, generation int, bootID string) (bool, error) {
	if !installerBootIDPattern.MatchString(bootID) {
		return false, errors.New("invalid reboot boot ID")
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRowContext(ctx, `UPDATE installer_reboot_remediations r SET scheduled_at=now(),last_error=$3||';'||r.last_error
 WHERE deployment_id=$1 AND generation=$2 AND state='SCHEDULED' AND last_error LIKE 'BOOTSTRAP_REPAIR_PENDING;%'
 AND EXISTS(SELECT 1 FROM provision_runs pr JOIN provision_step_attempts ps ON ps.run_id=pr.id
 WHERE pr.account_id=$4 AND pr.droplet_id=$5 AND ps.step='bootstrap-kernel-memory-guard'
 AND ps.last_started_at IS NOT NULL AND ps.last_finished_at>=ps.last_started_at AND ps.last_finished_at>r.scheduled_at AND ps.last_error IS NULL AND NOT ps.terminal)
 AND EXISTS(SELECT 1 FROM deployments d JOIN droplets dr ON dr.id=d.droplet_id JOIN accounts a ON a.id=d.account_id WHERE d.id=r.deployment_id AND d.account_id=$4 AND d.droplet_id=$5 AND d.installer_generation=$2 AND d.state='WAITING_INSTALLER' AND dr.state='PROVISIONING' AND a.deletion_requested_at IS NULL)
 RETURNING last_error`, d.ID, generation, bootstrapRepairReboot+";boot_id="+bootID, d.AccountID, d.DropletID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = (workflow.SQLStore{DB: tx}).Event(ctx, d.ID, "installer", workflow.WaitingInstaller, previous); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
