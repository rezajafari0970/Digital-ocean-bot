package app

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

func (c Container) deploymentInstallerRef(ctx context.Context, deploymentID string, snap workflow.ProfileSnapshot) (provisioning.InstallerRef, int, bool, error) {
	var ref provisioning.InstallerRef
	var generation int
	if err := c.DB.QueryRowContext(ctx, `SELECT installer_generation FROM deployments WHERE id=$1`, deploymentID).Scan(&generation); err != nil {
		return ref, 0, false, err
	}
	err := c.DB.QueryRowContext(ctx, `SELECT installer_name,installer_version FROM deployment_installer_selections WHERE deployment_id=$1 AND generation=$2`, deploymentID, generation).Scan(&ref.Name, &ref.Version)
	if err == nil {
		return ref, generation, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ref, generation, false, err
	}
	if generation == 1 && snap.InstallerRef != nil && snap.InstallerRef.Name != "" && snap.InstallerRef.Version > 0 {
		return *snap.InstallerRef, generation, true, nil
	}
	return ref, generation, false, nil
}
func (c Container) setInstallerDeploymentState(ctx context.Context, d workflow.Deployment, state workflow.State, step, msg string) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE deployments SET lock_version=lock_version+1,state=$3,current_step=$4,last_error=$5,updated_at=now() WHERE id=$1 AND account_id=$2 AND state='WAITING_INSTALLER'`, d.ID, d.AccountID, state, step, msg)
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
	if state == workflow.InstallFailed || state == workflow.InstallRolledBack {
		if _, err = tx.ExecContext(ctx, `UPDATE provision_runs SET state='FAILED',current_step=$3,last_error=$4,next_retry_at=NULL,updated_at=now() WHERE account_id=$1 AND droplet_id=$2 AND state<>'FAILED'`, d.AccountID, d.DropletID, step, msg); err != nil {
			return err
		}
		if err = (deploymentFailureFinalizer{DB: tx}).MarkFailed(ctx, d); err != nil {
			return err
		}
	}
	if err = (workflow.SQLStore{DB: tx}).Event(ctx, d.ID, "installer", state, msg); err != nil {
		return err
	}
	return tx.Commit()
}
func (c Container) installerTarget(ctx context.Context, d workflow.Deployment, snap workflow.ProfileSnapshot) (provisioning.Target, []byte, error) {
	if d.Host != "" {
		t := provisioning.Target{AccountID: d.AccountID, DropletID: d.DropletID, Host: d.Host, User: snap.SSHUser, KeySecretRef: snap.SSHKeySecretRef}
		key, err := c.Secrets.Get(ctx, d.AccountID, snap.SSHKeySecretRef)
		return t, key, err
	}
	runtime, err := c.Runtime(ctx, d.AccountID)
	if err != nil {
		return provisioning.Target{}, nil, err
	}
	compute, err := computeDriver(runtime)
	if err != nil {
		return provisioning.Target{}, nil, err
	}
	info, err := (workflow.ServerWaiter{Provider: compute}).Wait(ctx, d.ProviderID)
	if err != nil {
		return provisioning.Target{}, nil, err
	}
	t := provisioning.Target{AccountID: d.AccountID, DropletID: d.DropletID, Host: info.Host, User: snap.SSHUser, KeySecretRef: snap.SSHKeySecretRef}
	key, err := c.Secrets.Get(ctx, d.AccountID, snap.SSHKeySecretRef)
	return t, key, err
}
func (c Container) installerReadiness(ctx context.Context, d workflow.Deployment, runID string, target provisioning.Target, key []byte, ssh provisioning.SSHClient) (provisioning.ReadinessSnapshot, error) {
	// Readiness is volatile (package locks, DNS, HTTPS, reboot state). Always
	// collect a fresh observation for installer activation/retry; the recorder
	// keeps prior snapshots as history for diagnostics.
	store := provisioning.SQLStore{DB: c.DB}
	return (provisioning.ReadinessCollector{SSH: ssh, Recorder: store}).Collect(ctx, runID, target, key, nil)
}
func (c Container) activateInstaller(ctx context.Context, d workflow.Deployment, snap workflow.ProfileSnapshot) error {
	release, err := (workflow.PostgresRunLease{DB: c.DB}).Acquire(ctx, d.ID)
	if err != nil {
		return err
	}
	defer release()
	// Re-read under the same lease used by the initial provisioning worker.
	d, err = (workflow.SQLStore{DB: c.DB}).Get(ctx, d.ID, d.AccountID)
	if err != nil {
		return err
	}
	if d.State != workflow.WaitingInstaller {
		return nil
	}
	cfg, snap, err := c.DeploymentConfigFromSnapshot(ctx, d.ID)
	if err != nil {
		return err
	}
	eligible, err := c.reconcileInstallerBootstrap(ctx, d, cfg.Provision)
	if errors.Is(err, provisioning.ErrStepTerminal) {
		return c.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", err.Error())
	}
	if err != nil || !eligible {
		return err
	}
	if frozen, ferr := (RecoveryHandler{Container: c}).freezeExhaustedInstallerRecovery(ctx, d); ferr != nil {
		return ferr
	} else if frozen {
		return nil
	}
	ref, generation, ok, err := c.deploymentInstallerRef(ctx, d.ID, snap)
	if err != nil || !ok {
		return err
	}
	var runID string
	if err = c.DB.QueryRowContext(ctx, `SELECT id::text FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, d.AccountID, d.DropletID).Scan(&runID); err != nil {
		return err
	}
	var existingState string
	err = c.DB.QueryRowContext(ctx, `SELECT state FROM installer_runs WHERE deployment_id=$1 AND generation=$2`, d.ID, generation).Scan(&existingState)
	if err == nil && existingState == "INSTALL_COMPLETE" {
		return c.setInstallerDeploymentState(ctx, d, workflow.InstallComplete, "installer_complete", "")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	target, key, err := c.installerTarget(ctx, d, snap)
	if err != nil {
		return err
	}
	defer wipe(key)
	ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: c.DB}}
	ready, err := c.installerReadiness(ctx, d, runID, target, key, ssh)
	if err != nil {
		var re *provisioning.ReadinessError
		if errors.As(err, &re) && !re.Retryable {
			return c.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", err.Error())
		}
		return err
	}
	registry := provisioning.InstallerRegistry{DB: c.DB, Scripts: provisioning.ScriptRegistry{DB: c.DB}}
	orch := provisioning.InstallerOrchestrator{DB: c.DB, Registry: registry}
	resolved, ir, err := orch.Prepare(ctx, d.ID, runID, generation, ref, ready)
	if err != nil {
		if errors.Is(err, provisioning.ErrInstallerIncompatible) && ready.RebootRequired {
			remErr := c.remediateInstallerReboot(ctx, d, generation, target, key, ssh)
			if errors.Is(remErr, ErrInstallerRebootExhausted) {
				return c.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", remErr.Error())
			}
			return remErr
		}
		return err
	}
	switch ir.State {
	case "INSTALL_COMPLETE":
		return c.setInstallerDeploymentState(ctx, d, workflow.InstallComplete, "installer_complete", "")
	case "ROLLED_BACK":
		return c.setInstallerDeploymentState(ctx, d, workflow.InstallRolledBack, "installer_rolled_back", ir.LastError)
	case "FAILED":
		return c.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", ir.LastError)
	case "ROLLBACK_REQUIRED":
		if !resolved.Manifest.AutoRollback {
			return c.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", "rollback required")
		}
	}
	store := provisioning.SQLStore{DB: c.DB}
	exec := provisioning.InstallerExecutor{Store: store, Events: store, Scripts: provisioning.SSHScriptRunner{SSH: ssh}, States: orch}
	if ir.State == "ROLLBACK_REQUIRED" {
		if err = exec.Rollback(ctx, ir, resolved, target, key); err != nil {
			return err
		}
		return c.setInstallerDeploymentState(ctx, d, workflow.InstallRolledBack, "installer_rolled_back", "installer rolled back")
	}
	if err = exec.Execute(ctx, ir, resolved, target, key); err != nil {
		var state string
		_ = c.DB.QueryRowContext(ctx, `SELECT state FROM installer_runs WHERE id=$1`, ir.ID).Scan(&state)
		if state == "ROLLBACK_REQUIRED" && resolved.Manifest.AutoRollback {
			if rbErr := exec.Rollback(ctx, ir, resolved, target, key); rbErr != nil {
				return rbErr
			}
			return c.setInstallerDeploymentState(ctx, d, workflow.InstallRolledBack, "installer_rolled_back", "installer rolled back")
		}
		if state == "FAILED" {
			return c.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", err.Error())
		}
		return err
	}
	return c.setInstallerDeploymentState(ctx, d, workflow.InstallComplete, "installer_complete", "")
}

var ErrInstallerRebootScheduled = errors.New("installer reboot remediation scheduled")
var ErrInstallerRebootExhausted = errors.New("installer reboot remediation exhausted")

var installerBootIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func installerRebootBootID(marker string) string {
	for _, part := range strings.Split(marker, ";") {
		if strings.HasPrefix(part, "boot_id=") {
			id := strings.TrimPrefix(part, "boot_id=")
			if installerBootIDPattern.MatchString(id) {
				return id
			}
		}
	}
	return ""
}
func installerRebootCommand(bootID string) string {
	// The command reconciles a durable reboot intent. Even if a worker dies
	// before dispatch or loses the response, it cannot reboot a subsequent boot.
	return `if test "$(cat /proc/sys/kernel/random/boot_id)" = "` + bootID + `"; then systemctl reboot --no-block; fi`
}
func (c Container) remediateInstallerReboot(ctx context.Context, d workflow.Deployment, generation int, target provisioning.Target, key []byte, ssh provisioning.CommandRunner) error {
	var scheduledAt time.Time
	var state, marker string
	err := c.DB.QueryRowContext(ctx, `SELECT scheduled_at,state,last_error FROM installer_reboot_remediations WHERE deployment_id=$1 AND generation=$2`, d.ID, generation).Scan(&scheduledAt, &state, &marker)
	fresh := errors.Is(err, sql.ErrNoRows)
	if err != nil && !fresh {
		return err
	}
	repair := strings.HasPrefix(marker, bootstrapRepairPending+";")
	if fresh || repair {
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		bootID, probeErr := ssh.Run(probeCtx, target, key, "cat /proc/sys/kernel/random/boot_id")
		cancel()
		if probeErr != nil {
			return probeErr
		}
		bootID = strings.TrimSpace(bootID)
		if !installerBootIDPattern.MatchString(bootID) {
			return errors.New("invalid reboot boot ID")
		}
		if repair {
			claimed, claimErr := c.claimBootstrapRepairReboot(ctx, d, generation, bootID)
			if claimErr != nil {
				return claimErr
			}
			if !claimed {
				return errInstallerBootstrapEvidence
			}
		} else {
			res, insertErr := c.DB.ExecContext(ctx, `INSERT INTO installer_reboot_remediations(deployment_id,generation,state,last_error) VALUES($1,$2,'SCHEDULED',$3) ON CONFLICT(deployment_id,generation) DO NOTHING`, d.ID, generation, "INSTALLER_REBOOT;boot_id="+bootID)
			if insertErr != nil {
				return insertErr
			}
			n, rowsErr := res.RowsAffected()
			if rowsErr != nil {
				return rowsErr
			}
			if n != 1 {
				return ErrInstallerRebootScheduled
			}
		}
		marker = "INSTALLER_REBOOT;boot_id=" + bootID
	} else {
		if time.Since(scheduledAt) >= 3*time.Minute {
			return ErrInstallerRebootExhausted
		}
		// Historical entries have no boot proof; do not replay their command.
		if state != "SCHEDULED" || installerRebootBootID(marker) == "" {
			return ErrInstallerRebootScheduled
		}
	}
	bootID := installerRebootBootID(marker)
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err = ssh.Run(sendCtx, target, key, installerRebootCommand(bootID)); err != nil {
		// Keep the original boot proof on ambiguous SSH loss. Retry reconciles that
		// boot only, within the original budget; it does not grant another reboot.
		return err
	}
	return ErrInstallerRebootScheduled
}
