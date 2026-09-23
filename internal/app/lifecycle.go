package app

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

func (c Container) ProcessLifecycle(ctx context.Context, item droplets.LifecycleItem) error {
	if item.State == droplets.Expiring {
		if item.ProfileID == "" {
			return nil
		}
		if item.ReplacementDeploymentID == "" {
			var limit int
			_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE((data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, item.AccountID).Scan(&limit)
			var inUse int
			_ = c.DB.QueryRowContext(ctx, `SELECT count(*) FROM droplets WHERE account_id=$1 AND state NOT IN ('DELETED')`, item.AccountID).Scan(&inUse)
			var pending int
			_ = c.DB.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED')`, item.AccountID).Scan(&pending)
			if limit <= 0 || inUse+pending >= limit {
				_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ROTATION_BLOCKED_CAPACITY',runtime_status_detail=$2,runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID, fmt.Sprintf("droplet limit %d, in use %d, pending %d", limit, inUse, pending))
				return nil
			}
			_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,runtime_status_at=now(),updated_at=now() WHERE id=$1 AND runtime_status='ROTATION_BLOCKED_CAPACITY'`, item.AccountID)
			d, err := c.StartDeployment(ctx, item.AccountID, item.ProfileID)
			if err != nil {
				return err
			}
			_, err = c.DB.ExecContext(ctx, `UPDATE droplets SET replacement_deployment_id=$2,updated_at=now() WHERE id=$1 AND replacement_deployment_id IS NULL`, item.ID, d.ID)
			return err
		}
		var state string
		if err := c.DB.QueryRowContext(ctx, `SELECT state FROM deployments WHERE id=$1`, item.ReplacementDeploymentID).Scan(&state); err != nil {
			return err
		}
		if state != "READY" {
			return nil
		}
	}
	runtime, err := c.Runtime(ctx, item.AccountID)
	if err != nil {
		return err
	}
	executor := droplets.Executor{Operations: runtime.Operations, Provider: runtime.Provider, Gate: runtime.Gate}
	engine := droplets.LifecycleEngine{Store: droplets.LifecycleStore{DB: c.DB}, Executor: executor}
	return engine.Process(ctx, item)
}

func (c Container) ConfirmDeleted(ctx context.Context, accountID, providerID string) error {
	_, err := c.DB.ExecContext(ctx, `UPDATE droplets SET state='DELETED',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND state='DELETING'`, accountID, providerID)
	return err
}
