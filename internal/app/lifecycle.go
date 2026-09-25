package app

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"time"
)

func (c Container) ProcessLifecycle(ctx context.Context, item droplets.LifecycleItem) error {
	if item.State == droplets.Expiring {
		if item.ProfileID == "" {
			return nil
		}
		if item.ReplacementDeploymentID == "" {
			var limit, inUse, pending int
			var snapshotAt time.Time
			_ = c.DB.QueryRowContext(ctx, `SELECT
				COALESCE((SELECT (ps.data->'Limits'->>'DropletLimit')::int FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0),
				COALESCE((SELECT jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb)) FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0),
				(SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state IN ('planned','running','verifying','unknown') AND COALESCE(resource_id,'')=''),
				COALESCE((SELECT ps.created_at FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),'epoch'::timestamptz)`, item.AccountID).Scan(&limit, &inUse, &pending, &snapshotAt)
			if snapshotAt.Equal(time.Unix(0, 0)) || time.Since(snapshotAt) > 2*time.Minute {
				_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ROTATION_BLOCKED_CAPACITY',runtime_status_detail='provider snapshot stale; refresh required',runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID)
				return nil
			}
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
	if runtime.Gateway != nil &&
		!proxyAdapterByName(runtime.Config.ProxyAdapter).Capabilities().StickySession {

		eg := &network.EgressGuard{
			Client: runtime.Gateway.Client,
		}

		executor.EgressCheck = func(ctx context.Context) error {
			_, err := eg.Observe(ctx)
			return err
		}
	}
	engine := droplets.LifecycleEngine{Store: droplets.LifecycleStore{DB: c.DB}, Executor: executor}
	return engine.Process(ctx, item)
}

func (c Container) ConfirmDeleted(ctx context.Context, accountID, providerID string) error {
	_, err := c.DB.ExecContext(ctx, `UPDATE droplets SET state='DELETED',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND state='DELETING'`, accountID, providerID)
	return err
}
