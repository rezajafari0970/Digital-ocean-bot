package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"time"
)

type RecoveryHandler struct{ Container Container }

func (h RecoveryHandler) RecoverOperation(ctx context.Context, item worker.RecoveryItem) error {
	if item.Kind != "CREATE_DROPLET" && item.Kind != "DELETE_DROPLET" {
		return nil
	}
	var providerID string
	var operationCreated time.Time
	err := h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(resource_id,''),created_at FROM operations WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID).Scan(&providerID, &operationCreated)
	if err != nil {
		return err
	}
	if providerID == "" {
		if item.Kind != "CREATE_DROPLET" {
			return nil
		}
		var deploymentID, deploymentState string
		qerr := h.Container.DB.QueryRowContext(ctx, `SELECT d.id::text,d.state
			FROM deployments d
			JOIN operations o ON o.id=$1 AND o.account_id=$2
			WHERE o.idempotency_key LIKE 'deploy:'||d.id::text||':create:%'
			LIMIT 1`, item.ID, item.AccountID).Scan(&deploymentID, &deploymentState)
		if qerr != nil {
			if errors.Is(qerr, sql.ErrNoRows) {
				return nil
			}
			return qerr
		}
		runtime, rerr := h.Container.Runtime(ctx, item.AccountID)
		if rerr != nil {
			return rerr
		}
		compute, cerr := computeDriver(runtime)
		if cerr != nil {
			return cerr
		}
		matches, ferr := compute.FindServerByIdentity(ctx, "dob-deployment-"+deploymentID)
		if ferr != nil {
			return ferr
		}
		if len(matches) > 1 {
			return errors.New("multiple provider servers match stale create identity")
		}
		if len(matches) == 1 {
			_, err = h.Container.DB.ExecContext(ctx, `UPDATE operations SET resource_id=$3,state='verifying',updated_at=now() WHERE id=$1 AND account_id=$2 AND state IN ('running','unknown','verifying')`, item.ID, item.AccountID, matches[0].ID)
			return err
		}
		if terminalDeploymentStateForCreateRecovery(deploymentState) {
			_, err = h.Container.DB.ExecContext(ctx, `UPDATE operations SET state='failed',error_code='stale_create_without_provider_resource',updated_at=now() WHERE id=$1 AND account_id=$2 AND state IN ('running','unknown','verifying')`, item.ID, item.AccountID)
			return err
		}
		return nil
	}
	runtime, err := h.Container.Runtime(ctx, item.AccountID)
	if err != nil {
		return err
	}
	// Prefer canonical inventory evidence when a fresh provider observation exists.
	exists := false
	snapshotAuthoritative := false
	var snapAt time.Time
	if err := h.Container.DB.QueryRowContext(ctx, `SELECT created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, item.AccountID).Scan(&snapAt); err == nil && time.Since(snapAt) <= 2*time.Minute && snapAt.After(operationCreated) {
		var registryExists bool
		_ = h.Container.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM resources WHERE account_id=$1 AND provider_resource_id=$2 AND state<>'deleted')`, item.AccountID, providerID).Scan(&registryExists)
		exists = registryExists
		if item.Kind == "CREATE_DROPLET" && exists {
			snapshotAuthoritative = true
		}
	}
	if !snapshotAuthoritative {
		compute, cerr := computeDriver(runtime)
		if cerr != nil {
			return cerr
		}
		_, gerr := compute.GetServer(ctx, providerID)
		if gerr == nil {
			exists = true
		} else if providers.IsClass(gerr, providers.ErrorNotFound) {
			exists = false
		} else {
			return gerr
		}
	}
	state := "unknown"
	if item.Kind == "CREATE_DROPLET" && exists {
		state = "succeeded"
	}
	if item.Kind == "DELETE_DROPLET" && !exists {
		state = "succeeded"
	}
	_, err = h.Container.DB.ExecContext(ctx, `UPDATE operations SET state=$3,updated_at=now() WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID, state)
	if err != nil {
		return err
	}
	if item.Kind == "DELETE_DROPLET" && state == "succeeded" {
		if err := h.Container.ConfirmDeleted(ctx, item.AccountID, providerID); err != nil {
			return err
		}
		// The per-deployment public key is no longer needed by Vultr once the
		// server is gone. Remove it best-effort to avoid accumulating provider
		// SSH-key objects; deletion does not affect authorized_keys on the gone VM.
		var keyID string
		_ = h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(profile_snapshot->>'ssh_provider_key_id','') FROM deployments WHERE account_id=$1 AND provider_id=$2 ORDER BY created_at DESC LIMIT 1`, item.AccountID, providerID).Scan(&keyID)
		if keyID != "" {
			if sshDriver, ok := runtime.Driver.(providers.SSHKeyDriver); ok {
				if keyErr := sshDriver.DeleteSSHKey(ctx, keyID); keyErr == nil {
					_, _ = h.Container.DB.ExecContext(ctx, `UPDATE deployments SET profile_snapshot=jsonb_set(profile_snapshot,'{ssh_provider_key_deleted_at}',to_jsonb(now()::text),true),updated_at=now() WHERE account_id=$1 AND provider_id=$2`, item.AccountID, providerID)
				}
			}
		}
		return nil
	}
	return nil
}

func (h RecoveryHandler) RecoverDeployment(ctx context.Context, item worker.RecoveryItem) error {
	store := workflow.SQLStore{DB: h.Container.DB}
	d, err := store.Get(ctx, item.ID, item.AccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg, snap, err := h.Container.DeploymentConfigFromSnapshot(ctx, d.ID)
	if err != nil {
		return err
	}
	// A deployment reservation may survive a preparation failure. Before any
	// create-stage recovery, restore its per-deployment SSH identity so recovery
	// can never create a server without the persisted private-key reference.
	if d.CurrentStep == "create" && d.ProviderID == "" && (snap.SSHKeySecretRef == "" || snap.SSHProviderKeyID == "") {
		if err := h.Container.ensureDeploymentSSHIdentity(ctx, item.AccountID, d, &snap); err != nil {
			return err
		}
		cfg, snap, err = h.Container.DeploymentConfigFromSnapshot(ctx, d.ID)
		if err != nil {
			return err
		}
	}
	if d.State == workflow.InstallComplete || d.State == workflow.ImportingDatabase || d.State == workflow.DatabaseComplete || d.State == workflow.ConfiguringPanel {
		if err := h.Container.RequirePostInstallCapabilities(ctx, d.ID); err != nil {
			return err
		}
		engine, err := h.Container.PostInstallWorkflow(ctx, cfg)
		if err != nil {
			return err
		}
		_, err = engine.Run(ctx, workflow.Request{DeploymentID: d.ID, AccountID: item.AccountID, ProfileID: d.ProfileID, ClientCount: snap.ClientCount, InboundID: snap.InboundID, EmailPrefix: snap.EmailPrefix})
		return err
	}
	if d.State == workflow.WaitingInstaller {
		return h.Container.activateInstaller(ctx, d, snap)
	}
	if placeholder := provisioning.InstallerPlaceholderName(cfg.Provision); d.CurrentStep == "provision" && placeholder != "" {
		_, _ = h.Container.DB.ExecContext(ctx, `UPDATE deployments SET state='WAITING_INSTALLER',last_error='INSTALLER_NOT_CONFIGURED',updated_at=now() WHERE id=$1 AND account_id=$2`, d.ID, item.AccountID)
		_, _ = h.Container.DB.ExecContext(ctx, `UPDATE provision_runs SET state='WAITING_INSTALLER',current_step=$3,last_error='installer not configured',next_retry_at=NULL,updated_at=now() WHERE account_id=$1 AND droplet_id=$2`, item.AccountID, d.DropletID, placeholder)
		_ = store.Event(ctx, d.ID, "provision", workflow.WaitingInstaller, "installer not configured")
		return nil
	}
	if d.CurrentStep == "provision" && d.DropletID != "" {
		var ps, innerStep, pe string
		if qerr := h.Container.DB.QueryRowContext(ctx, `SELECT state,current_step,COALESCE(last_error,'') FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, item.AccountID, d.DropletID).Scan(&ps, &innerStep, &pe); qerr == nil {
			if ps == "FAILED" {
				_, _ = h.Container.DB.ExecContext(ctx, `UPDATE deployments SET state='FAILED',current_step='done',last_error=$3,updated_at=now() WHERE id=$1 AND account_id=$2`, d.ID, item.AccountID, "provision failed: "+pe)
				_ = store.Event(ctx, d.ID, "provision", workflow.Failed, "PROVISION_TERMINAL_FREEZE_V1")
				_ = (deploymentFailureFinalizer{DB: h.Container.DB}).MarkFailed(ctx, d)
				return nil
			}
			var pn sql.NullTime
			_ = h.Container.DB.QueryRowContext(ctx, `SELECT next_retry_at FROM provision_step_attempts psa JOIN provision_runs pr ON pr.id=psa.run_id WHERE pr.account_id=$1 AND pr.droplet_id=$2 AND psa.step=$3`, item.AccountID, d.DropletID, innerStep).Scan(&pn)
			if pn.Valid && time.Now().Before(pn.Time) {
				return nil
			}
		}
	}
	if d.CurrentStep == "create" {
		var opState, resourceID string
		qerr := h.Container.DB.QueryRowContext(ctx, `SELECT state,COALESCE(resource_id,'') FROM operations WHERE account_id=$1 AND idempotency_key LIKE 'deploy:'||$2||':create:%' ORDER BY created_at DESC LIMIT 1`, item.AccountID, d.ID).Scan(&opState, &resourceID)
		if qerr == nil {
			if opState == "failed" && resourceID == "" {
				_, _ = h.Container.DB.ExecContext(ctx, `UPDATE deployments SET state='FAILED',current_step='done',last_error='create operation failed; no provider resource created',updated_at=now() WHERE id=$1 AND account_id=$2`, d.ID, item.AccountID)
				_ = store.Event(ctx, d.ID, "create", workflow.Failed, "CREATE_TERMINAL_FREEZE_V2")
				return nil
			}
			// Unknown create outcomes with no resource_id must re-enter the create
			// step. The operation ledger returns the existing reservation and the
			// droplet executor performs tag-based adoption instead of issuing a
			// second provider create.
		}
	}
	engine, err := h.Container.Workflow(ctx, item.AccountID, cfg)
	if err != nil {
		return err
	}
	_, err = engine.Run(ctx, workflow.Request{DeploymentID: d.ID, AccountID: item.AccountID, ProfileID: d.ProfileID, ClientCount: snap.ClientCount, InboundID: snap.InboundID, EmailPrefix: snap.EmailPrefix})
	return err
}
func (h RecoveryHandler) BypassDeploymentBackoff(ctx context.Context, item worker.RecoveryItem) bool {
	var current string
	if err := h.Container.DB.QueryRowContext(ctx, `SELECT current_step FROM deployments WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID).Scan(&current); err != nil || current != "provision" {
		return false
	}
	var state string
	_ = h.Container.DB.QueryRowContext(ctx, `SELECT state FROM deployments WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID).Scan(&state)
	if state == string(workflow.WaitingInstaller) {
		var exists bool
		_ = h.Container.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM installer_runs ir JOIN deployments d ON d.id=ir.deployment_id WHERE ir.deployment_id=$1 AND ir.generation=d.installer_generation)`, item.ID).Scan(&exists)
		if exists {
			return false
		}
		var selected bool
		_ = h.Container.DB.QueryRowContext(ctx, `SELECT (profile_snapshot->'installer_ref' IS NOT NULL) OR EXISTS(SELECT 1 FROM deployment_installer_selections s WHERE s.deployment_id=deployments.id AND s.generation=deployments.installer_generation) FROM deployments WHERE id=$1`, item.ID).Scan(&selected)
		if selected {
			return true
		}
		return false
	}
	cfg, _, err := h.Container.DeploymentConfigFromSnapshot(ctx, item.ID)
	return err == nil && provisioning.PlanHasInstallerPlaceholder(cfg.Provision)
}

func terminalDeploymentStateForCreateRecovery(state string) bool {
	switch state {
	case "FAILED", "INSTALL_FAILED", "INSTALL_ROLLED_BACK", "PANEL_COMPLETE", "READY":
		return true
	default:
		return false
	}
}
