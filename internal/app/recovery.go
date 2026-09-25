package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
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
	err = h.Container.DB.QueryRowContext(ctx, `SELECT COALESCE(resource_id,'') FROM operations WHERE id=$1 AND account_id=$2`, item.ID, item.AccountID).Scan(&providerID)
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
	var snap []byte
	var snapAt time.Time
	if err := h.Container.DB.QueryRowContext(ctx, `SELECT data,created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, item.AccountID).Scan(&snap, &snapAt); err == nil && time.Since(snapAt) <= 2*time.Minute {
		var disc digitalocean.DiscoveryResult
		if json.Unmarshal(snap, &disc) == nil {
			for _, d := range disc.Droplets {
				if strconv.Itoa(d.ID) == providerID {
					exists = true
					break
				}
			}
		} else {
			snapAt = time.Time{}
		}
	}
	if snapAt.IsZero() || time.Since(snapAt) > 2*time.Minute {
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
	if d.CurrentStep == "provision" && d.DropletID != "" {
		var ps string
		var pa int
		var pn sql.NullTime
		var pe string
		if qerr := h.Container.DB.QueryRowContext(ctx, `SELECT state,attempt,next_retry_at,COALESCE(last_error,'') FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, item.AccountID, d.DropletID).Scan(&ps, &pa, &pn, &pe); qerr == nil {
			if ps == "FAILED" || pa >= 8 {
				_, _ = h.Container.DB.ExecContext(ctx, `UPDATE deployments SET state='FAILED',current_step='done',last_error=$3,updated_at=now() WHERE id=$1 AND account_id=$2`, d.ID, item.AccountID, "provision failed: "+pe)
				_ = store.Event(ctx, d.ID, "provision", workflow.Failed, "PROVISION_TERMINAL_FREEZE_V1")
				return nil
			}
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
			if opState == "unknown" && resourceID == "" {
				return nil
			}
		}
	}
	cfg, snap, err := h.Container.DeploymentConfigFromSnapshot(ctx, d.ID)
	if err != nil {
		return err
	}
	engine, err := h.Container.Workflow(ctx, item.AccountID, cfg)
	if err != nil {
		return err
	}
	_, err = engine.Run(ctx, workflow.Request{DeploymentID: d.ID, AccountID: item.AccountID, ProfileID: d.ProfileID, ClientCount: snap.ClientCount, InboundID: snap.InboundID, EmailPrefix: snap.EmailPrefix})
	return err
}
