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
func (c Container) activateInstaller(ctx context.Context, d workflow.Deployment, snap workflow.ProfileSnapshot) error {
	ref, ok, err := c.deploymentInstallerRef(ctx, d.ID, snap)
	if err != nil || !ok {
		return err
	}
	var ready provisioning.ReadinessSnapshot
	var checks []byte
	err = c.DB.QueryRowContext(ctx, `SELECT status,os_id,os_version,architecture,cpu_count,memory_mb,disk_free_mb,is_root,package_manager,package_health,dns_ok,outbound_https_ok,time_sync,reboot_required,checks FROM server_readiness_snapshots s JOIN provision_runs pr ON pr.id=s.run_id WHERE pr.account_id=$1 AND pr.droplet_id=$2 ORDER BY s.created_at DESC LIMIT 1`, d.AccountID, d.DropletID).Scan(&ready.Status, &ready.OSID, &ready.OSVersion, &ready.Architecture, &ready.CPUCount, &ready.MemoryMB, &ready.DiskFreeMB, &ready.IsRoot, &ready.PackageManager, &ready.PackageHealth, &ready.DNSOK, &ready.OutboundHTTPSOK, &ready.TimeSync, &ready.RebootRequired, &checks)
	if err != nil {
		return err
	}
	_ = json.Unmarshal(checks, &ready.Checks)
	var provisionRunID string
	if err = c.DB.QueryRowContext(ctx, `SELECT id::text FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, d.AccountID, d.DropletID).Scan(&provisionRunID); err != nil {
		return err
	}
	registry := provisioning.InstallerRegistry{DB: c.DB, Scripts: provisioning.ScriptRegistry{DB: c.DB}}
	orch := provisioning.InstallerOrchestrator{DB: c.DB, Registry: registry}
	resolved, ir, err := orch.Prepare(ctx, d.ID, provisionRunID, ref, ready)
	if err != nil {
		return err
	}
	if ir.State == "INSTALL_COMPLETE" {
		_, err = c.DB.ExecContext(ctx, `UPDATE deployments SET state='INSTALL_COMPLETE',current_step='installer_complete',last_error='',updated_at=now() WHERE id=$1 AND account_id=$2 AND state='WAITING_INSTALLER'`, d.ID, d.AccountID)
		return err
	}
	runtime, err := c.Runtime(ctx, d.AccountID)
	if err != nil {
		return err
	}
	info, err := (workflow.DigitalOceanWaiter{Provider: runtime.Provider}).Wait(ctx, d.ProviderID)
	if err != nil {
		return err
	}
	target := provisioning.Target{AccountID: d.AccountID, DropletID: d.DropletID, Host: info.Host, User: snap.SSHUser, KeySecretRef: snap.SSHKeySecretRef}
	key, err := c.Secrets.Get(ctx, d.AccountID, snap.SSHKeySecretRef)
	if err != nil {
		return err
	}
	ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: c.DB}}
	store := provisioning.SQLStore{DB: c.DB}
	exec := provisioning.InstallerExecutor{Store: store, Events: store, Scripts: provisioning.SSHScriptRunner{SSH: ssh}, States: orch}
	if err = exec.Execute(ctx, ir, resolved, target, key); err != nil {
		return err
	}
	_, err = c.DB.ExecContext(ctx, `UPDATE deployments SET state='INSTALL_COMPLETE',current_step='installer_complete',last_error='',updated_at=now() WHERE id=$1 AND account_id=$2 AND state='WAITING_INSTALLER'`, d.ID, d.AccountID)
	if err == nil {
		_ = (workflow.SQLStore{DB: c.DB}).Event(ctx, d.ID, "installer", workflow.InstallComplete, "installer complete")
	}
	return err
}
