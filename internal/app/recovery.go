package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

type RecoveryHandler struct{ Container Container }

func (h RecoveryHandler) RecoverOperation(ctx context.Context, item worker.RecoveryItem) (retErr error) {
	var version int64
	var versionLoaded bool
	defer func() {
		if retErr == nil || !versionLoaded {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cleanupCancel()
		if item.Kind == "DELETE_DROPLET" {
			var providerState string
			if err := h.Container.DB.QueryRowContext(cleanupCtx, `SELECT COALESCE(provider_state,'') FROM accounts WHERE id=$1`, item.AccountID).Scan(&providerState); err == nil {
				retErr = slowCleanupProviderError(providerState, retErr)
			}
		}
		code := string(providers.Class(retErr))
		if code == "" {
			code = "recovery_retry"
		}
		_, _ = h.Container.DB.ExecContext(cleanupCtx, `UPDATE operations SET updated_at=now(),error_code=$3,error_message=$4 WHERE id=$1 AND account_id=$2 AND state IN ('planned','running','unknown','verifying') AND lock_version=$5`, item.ID, item.AccountID, code, retErr.Error(), version)
	}()
	if item.Kind != "CREATE_DROPLET" && item.Kind != "DELETE_DROPLET" {
		return nil
	}
	var providerID string
	var operationCreated time.Time
	err := h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(resource_id,''),created_at,lock_version FROM operations WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID).Scan(&providerID, &operationCreated, &version)
	if err != nil {
		return err
	}
	versionLoaded = true
	if providerID == "" {
		if item.Kind == "DELETE_DROPLET" {
			result, err := h.Container.DB.ExecContext(ctx, `UPDATE operations SET state='failed',error_code='delete_without_provider_resource',error_message='delete recovery cannot continue without provider resource id',lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$3 AND state IN ('planned','running','unknown','verifying')`, item.ID, item.AccountID, version)
			return requireRecoveryOperationUpdate(result, err)
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
		runtime, rerr := h.Container.runtimeForLifecycle(ctx, item.AccountID)
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
		var deleting bool
		if err = h.Container.DB.QueryRowContext(ctx, "SELECT deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1", item.AccountID).Scan(&deleting); err != nil {
			return err
		}
		if deleting {
			if len(matches) == 0 {
				return errors.New("unknown create outcome on deleting account; absence is not final confirmation")
			}
			if time.Since(operationCreated) < 2*time.Minute {
				return errors.New("waiting for create request to settle")
			}
			if len(matches) == 1 {
				if err = h.Container.adoptDeletedAccountServer(ctx, item.AccountID, matches[0].ID); err != nil {
					return err
				}
			}
		}
		if len(matches) == 1 {
			result, err := h.Container.DB.ExecContext(ctx, `UPDATE operations SET resource_id=$3,state='verifying',error_code=NULL,error_message=NULL,lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$4 AND state IN ('running','unknown','verifying')`, item.ID, item.AccountID, matches[0].ID, version)
			return requireRecoveryOperationUpdate(result, err)
		}
		if terminalDeploymentStateForCreateRecovery(deploymentState) {
			result, err := h.Container.DB.ExecContext(ctx, `UPDATE operations SET state='failed',lock_version=lock_version+1,error_code='stale_create_without_provider_resource',updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$3 AND state IN ('running','unknown','verifying')`, item.ID, item.AccountID, version)
			return requireRecoveryOperationUpdate(result, err)
		}
		return nil
	}
	var runtime AccountRuntime
	if item.Kind == "DELETE_DROPLET" {
		runtime, err = h.Container.runtimeForLifecycle(ctx, item.AccountID)
	} else {
		runtime, err = h.Container.runtimeForLifecycle(ctx, item.AccountID)
	}
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
		if item.Kind == "DELETE_DROPLET" && exists {
			// DELETE is idempotent at the provider boundary. Re-issue the delete
			// instead of returning nil with state=unknown forever. Temporary
			// provider/network errors are returned so FailureStore applies backoff.
			if err := runtime.CheckMutationGeneration(ctx); err != nil {
				return err
			}
			if derr := compute.DeleteServer(ctx, providerID); derr != nil {
				return derr
			}
			result, err := h.Container.DB.ExecContext(ctx, `UPDATE operations SET state='verifying',attempt=attempt+1,error_code=NULL,error_message=NULL,lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$3 AND state IN ('running','unknown','verifying')`, item.ID, item.AccountID, version)
			return requireRecoveryOperationUpdate(result, err)
		}
	}
	state := "unknown"
	if item.Kind == "CREATE_DROPLET" && exists {
		var deleting bool
		if err = h.Container.DB.QueryRowContext(ctx, "SELECT deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1", item.AccountID).Scan(&deleting); err != nil {
			return err
		}
		if deleting {
			if err = h.Container.adoptDeletedAccountServer(ctx, item.AccountID, providerID); err != nil {
				return err
			}
		}
		state = "succeeded"
	}
	if item.Kind == "DELETE_DROPLET" && !exists {
		state = "succeeded"
	}
	if item.Kind == "DELETE_DROPLET" && state == "succeeded" {
		if err := h.Container.completeRecoveredDelete(ctx, item.ID, item.AccountID, providerID, version); err != nil {
			return err
		}
	} else {
		result, err := h.Container.DB.ExecContext(ctx, `UPDATE operations SET state=$3,lock_version=lock_version+1,error_code=CASE WHEN $3='succeeded' THEN NULL ELSE error_code END,error_message=CASE WHEN $3='succeeded' THEN NULL ELSE error_message END,updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$4 AND state IN ('running','unknown','verifying')`, item.ID, item.AccountID, state, version)
		if err := requireRecoveryOperationUpdate(result, err); err != nil {
			return err
		}
	}
	if item.Kind == "DELETE_DROPLET" && state == "succeeded" {
		// The per-deployment public key is no longer needed by Vultr once the
		// server is gone. Remove it best-effort to avoid accumulating provider
		// SSH-key objects; deletion does not affect authorized_keys on the gone VM.
		var keyID string
		_ = h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(profile_snapshot->>'ssh_provider_key_id','') FROM deployments WHERE account_id=$1 AND provider_id=$2 ORDER BY created_at DESC LIMIT 1`, item.AccountID, providerID).Scan(&keyID)
		if keyID != "" {
			if sshDriver, ok := runtime.Driver.(providers.SSHKeyDriver); ok {
				if err := runtime.CheckMutationGeneration(ctx); err != nil {
					return err
				}
				if keyErr := sshDriver.DeleteSSHKey(ctx, keyID); keyErr == nil {
					_, _ = h.Container.DB.ExecContext(ctx, `UPDATE deployments SET lock_version=lock_version+1,profile_snapshot=jsonb_set(profile_snapshot,'{ssh_provider_key_deleted_at}',to_jsonb(now()::text),true),updated_at=now() WHERE account_id=$1 AND provider_id=$2`, item.AccountID, providerID)
				}
			}
		}
		return nil
	}
	return nil
}

func (h RecoveryHandler) RecoverDeployment(ctx context.Context, item worker.RecoveryItem) error {
	var deleting bool
	if err := h.Container.DB.QueryRowContext(ctx, "SELECT deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1", item.AccountID).Scan(&deleting); err != nil {
		return err
	}
	if deleting {
		return nil
	}

	store := workflow.SQLStore{DB: h.Container.DB}
	d, err := store.Get(ctx, item.ID, item.AccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if stopped, err := h.expireInitialSSH(ctx, d); err != nil || stopped {
		return err
	}
	// Identity preparation is covered by the execution lease too: two recovery
	// runners must never upload different keys for the same reserved deployment.
	release, err := (workflow.PostgresRunLease{DB: h.Container.DB}).Acquire(ctx, d.ID)
	if errors.Is(err, workflow.ErrDeploymentBusy) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	d, err = store.Get(ctx, item.ID, item.AccountID)
	if err != nil {
		return err
	}
	if terminalDeploymentStateForCreateRecovery(string(d.State)) {
		return nil
	}
	cfg, snap, err := h.Container.DeploymentConfigFromSnapshot(ctx, d.ID)
	if err != nil {
		return err
	}
	// A deployment reservation may survive a preparation failure. Before any
	// create-stage recovery, restore its per-deployment SSH identity so recovery
	// can never create a server without the persisted private-key reference.
	if d.CurrentStep == "create" && d.ProviderID == "" && (snap.SSHKeySecretRef == "" || (snap.SSHProviderKeyID == "" && snap.SSHAuthorizedKey == "")) {
		if err := h.Container.ensureDeploymentSSHIdentity(ctx, item.AccountID, d, &snap); err != nil {
			if frozen, ferr := h.freezePermanentDeploymentError(ctx, d, err); ferr != nil {
				return ferr
			} else if frozen {
				return nil
			}
			return err
		}
		cfg, snap, err = h.Container.DeploymentConfigFromSnapshot(ctx, d.ID)
		if err != nil {
			return err
		}
	}
	release()
	release = nil
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
	if d.CurrentStep == "provision" && d.DropletID != "" {
		var ps, innerStep, pe string
		if qerr := h.Container.DB.QueryRowContext(ctx, `SELECT state,current_step,COALESCE(last_error,'') FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, item.AccountID, d.DropletID).Scan(&ps, &innerStep, &pe); qerr == nil {
			if ps == "FAILED" {
				return h.persistTerminalRecovery(ctx, d, "provision failed: "+pe, "PROVISION_TERMINAL_FREEZE_V1")
			}
			pn, err := provisionStepRetryAt(ctx, h.Container.DB, item.AccountID, d.DropletID, innerStep)
			if err != nil {
				return err
			}
			if pn.Valid && time.Now().Before(pn.Time) {
				return nil
			}
		} else if !errors.Is(qerr, sql.ErrNoRows) {
			return qerr
		}
	}
	if d.CurrentStep == "create" {
		var opState, resourceID string
		qerr := h.Container.DB.QueryRowContext(ctx, `SELECT state,COALESCE(resource_id,'') FROM operations WHERE account_id=$1 AND idempotency_key LIKE 'deploy:'||$2||':create:%' ORDER BY created_at DESC LIMIT 1`, item.AccountID, d.ID).Scan(&opState, &resourceID)
		if qerr == nil {
			if opState == "failed" && resourceID == "" {
				return h.persistTerminalRecovery(ctx, d, "create operation failed; no provider resource created", "CREATE_TERMINAL_FREEZE_V2")
			}
			// Unknown create outcomes with no resource_id must re-enter the create
			// step. The operation ledger returns the existing reservation and the
			// droplet executor performs tag-based adoption instead of issuing a
			// second provider create.
		}
	}
	engine, err := h.Container.Workflow(ctx, item.AccountID, cfg)
	if err != nil {
		if frozen, ferr := h.freezePermanentDeploymentError(ctx, d, err); ferr != nil {
			return ferr
		} else if frozen {
			return nil
		}
		return err
	}
	_, err = engine.Run(ctx, workflow.Request{DeploymentID: d.ID, AccountID: item.AccountID, ProfileID: d.ProfileID, ClientCount: snap.ClientCount, InboundID: snap.InboundID, EmailPrefix: snap.EmailPrefix})
	if err != nil {
		if frozen, ferr := h.freezePermanentDeploymentError(ctx, d, err); ferr != nil {
			return ferr
		} else if frozen {
			return nil
		}
	}
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

func (h RecoveryHandler) persistTerminalRecovery(ctx context.Context, d workflow.Deployment, msg, code string) error {
	tx, err := h.Container.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	d.State = workflow.Failed
	d.CurrentStep = "done"
	d.LastError = msg
	store := workflow.SQLStore{DB: tx}
	if err = store.Update(ctx, &d); err != nil {
		return err
	}
	if err = (deploymentFailureFinalizer{DB: tx}).MarkFailed(ctx, d); err != nil {
		return err
	}
	if err = store.Event(ctx, d.ID, "recovery", workflow.Failed, code); err != nil {
		return err
	}
	return tx.Commit()
}

func terminalDeploymentStateForCreateRecovery(state string) bool {
	switch state {
	case "FAILED", "INSTALL_FAILED", "INSTALL_ROLLED_BACK", "PANEL_COMPLETE", "READY":
		return true
	default:
		return false
	}
}

func (h RecoveryHandler) freezePermanentDeploymentError(ctx context.Context, d workflow.Deployment, err error) (bool, error) {
	class := providers.Class(err)
	permanent := class == providers.ErrorBilling || class == providers.ErrorAuthentication || class == providers.ErrorPermissionDenied || class == providers.ErrorInvalidRequest || class == providers.ErrorImageUnavailable
	if !permanent && d.CurrentStep == "create" && d.ProviderID == "" {
		var providerState string
		if qerr := h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(provider_state,'') FROM accounts WHERE id=$1`, d.AccountID).Scan(&providerState); qerr == nil {
			permanent = providerState == ProviderStateTokenInvalid || providerState == ProviderStatePermissionDenied || providerState == ProviderStateBillingBlocked || providerState == ProviderStateLocked
		}
	}
	if !permanent {
		return false, nil
	}
	msg := err.Error()
	if class != providers.ErrorUnknown && class != "" {
		state := ClassifyAccountProviderError(err, false)
		if state != ProviderStateTransportError {
			h.Container.RecordProviderObservation(ctx, d.AccountID, state, err, msg)
		}
	}
	if terminalDeploymentStateForCreateRecovery(string(d.State)) {
		return false, nil
	}
	if err := h.persistTerminalRecovery(ctx, d, msg, "RECOVERY_PERMANENT_PROVIDER_ERROR:"+string(class)); err != nil {
		return false, err
	}
	return true, nil
}

func (h RecoveryHandler) freezeExhaustedInstallerRecovery(ctx context.Context, d workflow.Deployment) (bool, error) {
	if d.State != workflow.WaitingInstaller {
		return false, nil
	}
	var failures int
	var firstFailed sql.NullTime
	var lastErr string
	err := h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(f.failures,0),f.first_failed_at,COALESCE(f.last_error,'') FROM worker_item_failures f WHERE f.kind='deployment' AND f.item_id=$1`, d.ID).Scan(&failures, &firstFailed, &lastErr)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	exhausted := failures >= 64
	if firstFailed.Valid {
		budget := 30 * time.Minute
		if strings.Contains(lastErr, "SSH_CONNECTION_REFUSED") {
			budget = 15 * time.Minute
		}
		if time.Since(firstFailed.Time) >= budget {
			exhausted = true
		}
	}
	if !exhausted {
		return false, nil
	}
	if lastErr == "" {
		lastErr = "installer recovery budget exhausted"
	}
	return true, h.Container.setInstallerDeploymentState(ctx, d, workflow.InstallFailed, "installer_failed", "recovery budget exhausted: "+lastErr)
}

// A missing step is not a retry delay. A failed read cannot authorize execution.
func provisionStepRetryAt(ctx context.Context, db *sql.DB, account, droplet, step string) (sql.NullTime, error) {
	var next sql.NullTime
	err := db.QueryRowContext(ctx, `SELECT psa.next_retry_at FROM provision_step_attempts psa JOIN provision_runs pr ON pr.id=psa.run_id WHERE pr.account_id=$1 AND pr.droplet_id=$2 AND psa.step=$3`, account, droplet, step).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullTime{}, nil
	}
	return next, err
}

// A lost compare-and-swap is not durable completion. The next fenced attempt
// reads the current native/SQL state before attempting further progress.
func requireRecoveryOperationUpdate(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("operation changed during recovery")
	}
	return nil
}
