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
		// A failed deployment is not serving traffic, so requiring a replacement
		// before deletion creates a capacity deadlock (especially at the limit).
		// Only healthy/nonterminal service resources require replacement-first.
		var deploymentState string
		_ = c.DB.QueryRowContext(ctx, `SELECT state FROM deployments WHERE droplet_id=$1 ORDER BY updated_at DESC LIMIT 1`, item.ID).Scan(&deploymentState)
		terminalFailed := deploymentState == "FAILED" || deploymentState == "INSTALL_FAILED" || deploymentState == "INSTALL_ROLLED_BACK"
		if !terminalFailed && item.ProfileID == "" {
			return nil
		}
		if !terminalFailed && item.ReplacementDeploymentID == "" {
			// Rotation must share the same concurrency budget as the scheduler.
			// Otherwise overlapping expiry windows can create a replacement burst.
			var active, maxConcurrent int
			if err := c.DB.QueryRowContext(ctx, `SELECT
				(SELECT count(*) FROM deployments d WHERE d.account_id=$1 AND d.state IN ('RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL') AND (d.droplet_id IS NULL OR EXISTS(SELECT 1 FROM droplets r WHERE r.id=d.droplet_id AND r.state<>'DELETED'))),
				COALESCE((SELECT NULLIF(max_concurrent,0) FROM schedules WHERE account_id=$1 AND profile_id=$2 ORDER BY created_at DESC LIMIT 1),1)`, item.AccountID, item.ProfileID).Scan(&active, &maxConcurrent); err != nil {
				return err
			}
			if active >= maxConcurrent {
				return nil
			}
			var limit, inUse, pending int
			var providerState, providerError string
			var snapshotAt time.Time
			_ = c.DB.QueryRowContext(ctx, `SELECT
				COALESCE((SELECT (ps.data->'Limits'->>'DropletLimit')::int FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0),
				COALESCE((SELECT jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb)) FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0),
				(SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state IN ('planned','running','verifying','unknown') AND COALESCE(resource_id,'')=''),
				COALESCE((SELECT ps.created_at FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),'epoch'::timestamptz),
				COALESCE((SELECT provider_state FROM accounts WHERE id=$1),'UNKNOWN'),COALESCE((SELECT provider_error_state FROM accounts WHERE id=$1),'')`, item.AccountID).Scan(&limit, &inUse, &pending, &snapshotAt, &providerState, &providerError)
			if providerState != "ACTIVE" || providerError != "" {
				return nil
			}
			if snapshotAt.Equal(time.Unix(0, 0)) || time.Since(snapshotAt) > 2*time.Minute {
				_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ROTATION_BLOCKED_CAPACITY',runtime_status_detail='provider snapshot stale; refresh required',runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID)
				return nil
			}
			if limit <= 0 || inUse+pending >= limit {
				var runtimeStatus, oldestExpiring string
				_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(runtime_status,''),(SELECT id::text FROM droplets WHERE account_id=$1 AND state='EXPIRING' ORDER BY created_at,id LIMIT 1) FROM accounts WHERE id=$1`, item.AccountID).Scan(&runtimeStatus, &oldestExpiring)
				if limit > 0 && inUse >= limit && pending == 0 && oldestExpiring == item.ID && runtimeStatus != "ROTATION_CAPACITY_BREAKING" {
					_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ROTATION_CAPACITY_BREAKING',runtime_status_detail='deleting one oldest expiring server to free one replacement slot',runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID)
					// Continue to LifecycleEngine below: exactly one oldest EXPIRING resource may be removed.
				} else {
					_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status=CASE WHEN runtime_status='ROTATION_CAPACITY_BREAKING' THEN runtime_status ELSE 'ROTATION_BLOCKED_CAPACITY' END,runtime_status_detail=$2,runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID, fmt.Sprintf("droplet limit %d, in use %d, pending %d", limit, inUse, pending))
					return nil
				}
			} else {
				_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,runtime_status_at=now(),updated_at=now() WHERE id=$1 AND runtime_status IN ('ROTATION_BLOCKED_CAPACITY','ROTATION_CAPACITY_BREAKING')`, item.AccountID)
			}
			if inUse+pending < limit {
				// Capacity is available; proceed with the normal replacement-first path.
			} else {
				// Capacity-breaker path: skip replacement creation and delete only this oldest EXPIRING item.
				goto processLifecycle
			}
			d, err := c.StartDeployment(ctx, item.AccountID, item.ProfileID)
			if err != nil {
				return err
			}
			_, err = c.DB.ExecContext(ctx, `UPDATE droplets SET replacement_deployment_id=$2,updated_at=now() WHERE id=$1 AND replacement_deployment_id IS NULL`, item.ID, d.ID)
			return err
		}
		if !terminalFailed {
			var replacementReady bool
			if err := c.DB.QueryRowContext(ctx, `SELECT EXISTS(
				SELECT 1 FROM deployments d JOIN droplets r ON r.id=d.droplet_id
				WHERE d.id=$1 AND d.state='PANEL_COMPLETE' AND r.state='READY'
			)`, item.ReplacementDeploymentID).Scan(&replacementReady); err != nil {
				return err
			}
			if !replacementReady {
				return nil
			}
		}
	}
processLifecycle:
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
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE droplets SET state='DELETED',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND state IN ('DELETING','DELETED')`, accountID, providerID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE resources SET state='deleted',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND managed=true`, accountID, providerID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `UPDATE deployments SET state='FAILED',current_step='done',last_error='RESOURCE_DELETED',updated_at=now() WHERE account_id=$1 AND provider_id=$2 AND state IN ('RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL') RETURNING id::text`, accountID, providerID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,'lifecycle','FAILED','RESOURCE_DELETED')`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
