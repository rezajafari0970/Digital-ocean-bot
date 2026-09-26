package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

func (c Container) deploymentInstallerRef(ctx context.Context, deploymentID string, snap workflow.ProfileSnapshot) (provisioning.InstallerRef, bool, error) {
	if snap.InstallerRef != nil && snap.InstallerRef.Name != "" && snap.InstallerRef.Version > 0 {
		return *snap.InstallerRef, true, nil
	}
	var ref provisioning.InstallerRef
	err := c.DB.QueryRowContext(ctx, `SELECT installer_name,installer_version FROM deployment_installer_selections WHERE deployment_id=$1`, deploymentID).Scan(&ref.Name, &ref.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return ref, false, nil
	}
	return ref, err == nil, err
}
func (c Container) setInstallerDeploymentState(ctx context.Context, d workflow.Deployment, state workflow.State, step, msg string) error {
	_, err := c.DB.ExecContext(ctx, `UPDATE deployments SET state=$3,current_step=$4,last_error=$5,updated_at=now() WHERE id=$1 AND account_id=$2 AND state='WAITING_INSTALLER'`, d.ID, d.AccountID, state, step, msg)
	if err == nil {
		_ = (workflow.SQLStore{DB: c.DB}).Event(ctx, d.ID, "installer", state, msg)
	}
	return err
}
func (c Container) installerTarget(ctx context.Context, d workflow.Deployment, snap workflow.ProfileSnapshot) (provisioning.Target, []byte, error) {
	runtime, err := c.Runtime(ctx, d.AccountID)
	if err != nil {
		return provisioning.Target{}, nil, err
	}
	info, err := (workflow.DigitalOceanWaiter{Provider: runtime.Provider}).Wait(ctx, d.ProviderID)
	if err != nil {
		return provisioning.Target{}, nil, err
	}
	t := provisioning.Target{AccountID: d.AccountID, DropletID: d.DropletID, Host: info.Host, User: snap.SSHUser, KeySecretRef: snap.SSHKeySecretRef}
	key, err := c.Secrets.Get(ctx, d.AccountID, snap.SSHKeySecretRef)
	return t, key, err
}
func (c Container) installerReadiness(ctx context.Context, d workflow.Deployment, runID string, target provisioning.Target, key []byte, ssh provisioning.SSHClient) (provisioning.ReadinessSnapshot, error) {
	var ready provisioning.ReadinessSnapshot
	var checks []byte
	err := c.DB.QueryRowContext(ctx, `SELECT status,os_id,os_version,architecture,cpu_count,memory_mb,disk_free_mb,is_root,package_manager,package_health,dns_ok,outbound_https_ok,time_sync,reboot_required,checks FROM server_readiness_snapshots WHERE run_id=$1 ORDER BY created_at DESC LIMIT 1`, runID).Scan(&ready.Status, &ready.OSID, &ready.OSVersion, &ready.Architecture, &ready.CPUCount, &ready.MemoryMB, &ready.DiskFreeMB, &ready.IsRoot, &ready.PackageManager, &ready.PackageHealth, &ready.DNSOK, &ready.OutboundHTTPSOK, &ready.TimeSync, &ready.RebootRequired, &checks)
	if err == nil {
		_ = json.Unmarshal(checks, &ready.Checks)
		return ready, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ready, err
	}
	store := provisioning.SQLStore{DB: c.DB}
	return (provisioning.ReadinessCollector{SSH: ssh, Recorder: store}).Collect(ctx, runID, target, key, nil)
}
func (c Container) activateInstaller(ctx context.Context, d workflow.Deployment, snap workflow.ProfileSnapshot) error {
	ref, ok, err := c.deploymentInstallerRef(ctx, d.ID, snap)
	if err != nil || !ok {
		return err
	}
	var runID string
	if err = c.DB.QueryRowContext(ctx, `SELECT id::text FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, d.AccountID, d.DropletID).Scan(&runID); err != nil {
		return err
	}
	target, key, err := c.installerTarget(ctx, d, snap)
	if err != nil {
		return err
	}
	ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: c.DB}}
	ready, err := c.installerReadiness(ctx, d, runID, target, key, ssh)
	if err != nil {
		return err
	}
	registry := provisioning.InstallerRegistry{DB: c.DB, Scripts: provisioning.ScriptRegistry{DB: c.DB}}
	orch := provisioning.InstallerOrchestrator{DB: c.DB, Registry: registry}
	resolved, ir, err := orch.Prepare(ctx, d.ID, runID, ref, ready)
	if err != nil {
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
