package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"strconv"
	"time"
)

type RecoveryHandler struct{ Container Container }

func (h RecoveryHandler) RecoverOperation(ctx context.Context, item worker.RecoveryItem) error {
	runtime, err := h.Container.Runtime(ctx, item.AccountID)
	if err != nil {
		return err
	}
	if item.Kind != "CREATE_DROPLET" && item.Kind != "DELETE_DROPLET" {
		return nil
	}
	var providerID string
	var operationCreated time.Time
	err = h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(resource_id,''),created_at FROM operations WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID).Scan(&providerID, &operationCreated)
	if err != nil {
		return err
	}
	if providerID == "" {
		return nil
	}
	// Recovery is the only path allowed to query the provider for an uncertain
	// mutation outcome. Prefer the latest provider snapshot when it is fresh;
	// only fall back to a live provider request when local evidence is stale.
	exists := false
	snapshotAuthoritative := false
	var snap []byte
	var snapAt time.Time
	if err := h.Container.DB.QueryRowContext(ctx, `SELECT data,created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, item.AccountID).Scan(&snap, &snapAt); err == nil && time.Since(snapAt) <= 2*time.Minute && snapAt.After(operationCreated) {
		var disc digitalocean.DiscoveryResult
		if json.Unmarshal(snap, &disc) == nil {
			for _, d := range disc.Droplets {
				if strconv.Itoa(d.ID) == providerID {
					exists = true
					break
				}
			}
			// Presence is authoritative for CREATE. Absence is authoritative for
			// DELETE. The opposite direction requires a live provider check: a
			// snapshot may have been captured while the mutation was still settling.
			if item.Kind == "CREATE_DROPLET" && exists {
				snapshotAuthoritative = true
			}
			if item.Kind == "DELETE_DROPLET" && !exists {
				snapshotAuthoritative = true
			}
		}
	}
	if !snapshotAuthoritative {
		pid, convErr := strconv.Atoi(providerID)
		if convErr != nil {
			return convErr
		}
		exists, err = runtime.Provider.DropletExists(ctx, pid)
		if err != nil {
			return err
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
		return h.Container.ConfirmDeleted(ctx, item.AccountID, providerID)
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
	cfg, _, err := h.Container.DeploymentConfigFromSnapshot(ctx, item.ID)
	return err == nil && provisioning.PlanHasInstallerPlaceholder(cfg.Provision)
}
