package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
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
	resources, err := runtime.Provider.ListDroplets(ctx)
	if err != nil {
		return err
	}
	exists := false
	for _, r := range resources {
		if r.ID == providerID {
			exists = true
			break
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
	return err
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
	engine, err := h.Container.Workflow(ctx, item.AccountID, cfg)
	if err != nil {
		return err
	}
	_, err = engine.Run(ctx, workflow.Request{AccountID: item.AccountID, ProfileID: d.ProfileID, ClientCount: snap.ClientCount, InboundID: snap.InboundID, EmailPrefix: snap.EmailPrefix})
	return err
}
