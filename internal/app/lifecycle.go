package app

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"time"
)

func (c Container) ProcessLifecycle(ctx context.Context, item droplets.LifecycleItem) error {
	var providerState string
	_ = c.DB.QueryRowContext(ctx, "SELECT COALESCE(provider_state,'') FROM accounts WHERE id=$1", item.AccountID).Scan(&providerState)
	providerLocked := providerState == ProviderStateLocked
	if providerLocked {
		// A locked provider account is never eligible for replacement or serving.
		// Move READY/EXPIRING resources toward retirement locally; physical DELETE
		// still uses the normal account network/gate and requires provider confirmation.
		_, _ = c.DB.ExecContext(ctx, "UPDATE resources SET state='retiring',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND managed=true AND state<>'deleted'", item.AccountID, item.ProviderID)
		if item.State == droplets.Ready || item.State == droplets.Expiring {
			ok, err := (droplets.LifecycleStore{DB: c.DB}).Transition(ctx, item.ID, item.State, droplets.Retiring)
			if err != nil {
				return err
			}
			if ok {
				item.State = droplets.Retiring
				if err = (droplets.LifecycleStore{DB: c.DB}).Event(ctx, item, droplets.Retiring); err != nil {
					return err
				}
			}
		}
	}
	if item.State == droplets.Expiring && item.ReplacementDeploymentID != "" {
		var replacementState string
		_ = c.DB.QueryRowContext(ctx, "SELECT state FROM deployments WHERE id=$1 AND account_id=$2", item.ReplacementDeploymentID, item.AccountID).Scan(&replacementState)
		if replacementState == "FAILED" || replacementState == "INSTALL_FAILED" || replacementState == "INSTALL_ROLLED_BACK" {
			_, _ = c.DB.ExecContext(ctx, "UPDATE droplets SET replacement_deployment_id=NULL,updated_at=now() WHERE id=$1 AND replacement_deployment_id=$2", item.ID, item.ReplacementDeploymentID)
			item.ReplacementDeploymentID = ""
		}
	}
	if item.State == droplets.Expiring {
		// Converge oversupply back to desired capacity. Claim excess retirement
		// under an account-scoped advisory lock so concurrent lifecycle workers
		// cannot retire more resources than the current excess.
		if item.ReplacementDeploymentID == "" {
			tx, err := c.DB.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "lifecycle-excess:"+item.AccountID); err != nil {
				tx.Rollback()
				return err
			}
			var desired, managed, retiring int
			if err = tx.QueryRowContext(ctx, `SELECT a.desired_server_count,
				(SELECT count(*) FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED'),
				(SELECT count(*) FROM droplets d WHERE d.account_id=a.id AND d.state IN ('RETIRING','DELETING'))
				FROM accounts a WHERE a.id=$1`, item.AccountID).Scan(&desired, &managed, &retiring); err != nil {
				tx.Rollback()
				return err
			}
			claimed := false
			if desired > 0 && managed > desired && retiring < managed-desired {
				res, qerr := tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND state='EXPIRING'`, item.ID)
				if qerr != nil {
					tx.Rollback()
					return qerr
				}
				n, _ := res.RowsAffected()
				claimed = n == 1
			} else if desired > 0 && managed >= desired && retiring == 0 {
				// Hard Desired is a strict ceiling. At the ceiling, replacement-first
				// would require a temporary (desired+1) server and deadlock against
				// StartDeployment admission. Retire exactly one oldest EXPIRING
				// server first; the scheduler then backfills the freed slot.
				var oldest string
				_ = tx.QueryRowContext(ctx, `SELECT id::text FROM droplets WHERE account_id=$1 AND state='EXPIRING' ORDER BY expires_at NULLS LAST,created_at,id LIMIT 1`, item.AccountID).Scan(&oldest)
				if shouldDeleteFirstAtDesired(desired, managed, retiring, oldest == item.ID) {
					res, qerr := tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND state='EXPIRING'`, item.ID)
					if qerr != nil {
						tx.Rollback()
						return qerr
					}
					n, _ := res.RowsAffected()
					claimed = n == 1
				}
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			if claimed {
				item.State = droplets.Retiring
				_ = (droplets.LifecycleStore{DB: c.DB}).Event(ctx, item, droplets.Retiring)
				goto processLifecycle
			}
			// At/above the hard Desired ceiling, another retirement/deletion is
			// already sufficient to free the next slot. Other EXPIRING items wait
			// instead of attempting replacement deployments that admission must reject.
			if desired > 0 && managed >= desired {
				return nil
			}
		}
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
				(SELECT count(*) FROM deployments d WHERE d.account_id=$1 AND d.state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL','REGISTERING_CLIENTS','REGISTERING_TRAFFIC') AND (d.droplet_id IS NULL OR EXISTS(SELECT 1 FROM droplets r WHERE r.id=d.droplet_id AND r.state<>'DELETED'))),
				COALESCE((SELECT NULLIF(max_concurrent,0) FROM schedules WHERE account_id=$1 AND profile_id=$2 ORDER BY created_at DESC LIMIT 1),1)`, item.AccountID, item.ProfileID).Scan(&active, &maxConcurrent); err != nil {
				return err
			}
			if active >= maxConcurrent {
				return nil
			}
			var providerState, providerError string
			_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(provider_state,'UNKNOWN'),COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, item.AccountID).Scan(&providerState, &providerError)
			cap, capErr := capacity.Read(ctx, c.DB, item.AccountID, 2*time.Minute)
			limit, inUse, pending := cap.Limit, cap.InUse, cap.Pending
			if providerState != "ACTIVE" || providerError != "" {
				return nil
			}
			if capErr != nil {
				_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ROTATION_BLOCKED_CAPACITY',runtime_status_detail='provider snapshot stale; refresh required',runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID)
				return nil
			}
			if cap.LimitKnown && (limit <= 0 || inUse+pending >= limit) {
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
				_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,runtime_status_at=now(),updated_at=now() WHERE id=$1 AND provider_state='ACTIVE' AND COALESCE(provider_error_state,'')='' AND runtime_status IN ('ROTATION_BLOCKED_CAPACITY','ROTATION_CAPACITY_BREAKING')`, item.AccountID)
			}
			if !cap.LimitKnown || inUse+pending < limit {
				// Capacity is available, or the provider does not publish a hard limit.
			} else {
				// Capacity-breaker path: skip replacement creation and delete only this oldest EXPIRING item.
				goto processLifecycle
			}
			// Claim replacement ownership before any provider mutation. The advisory
			// transaction lock serializes contenders for this exact lifecycle item.
			claimTx, err := c.DB.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if _, err = claimTx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "replacement-claim:"+item.ID); err != nil {
				claimTx.Rollback()
				return err
			}
			var existing string
			if err = claimTx.QueryRowContext(ctx, `SELECT COALESCE(replacement_deployment_id::text,'') FROM droplets WHERE id=$1 FOR UPDATE`, item.ID).Scan(&existing); err != nil {
				claimTx.Rollback()
				return err
			}
			if existing != "" {
				return claimTx.Commit()
			}
			// Keep the claim transaction open while StartDeployment performs its short
			// account admission/reservation. No provider mutation occurs before that
			// reservation is durable; contenders block on this droplet claim.
			d, err := c.StartDeployment(ctx, item.AccountID, item.ProfileID)
			if err != nil {
				claimTx.Rollback()
				return err
			}
			if _, err = claimTx.ExecContext(ctx, `UPDATE droplets SET replacement_deployment_id=$2,updated_at=now() WHERE id=$1 AND replacement_deployment_id IS NULL`, item.ID, d.ID); err != nil {
				claimTx.Rollback()
				return err
			}
			return claimTx.Commit()
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
	compute, err := computeDriver(runtime)
	if err != nil {
		return err
	}
	executor := droplets.Executor{Operations: runtime.Operations, Provider: compute, Gate: runtime.Gate}
	var eg *network.EgressGuard
	if runtime.Gateway != nil && !proxyAdapterByName(runtime.Config.ProxyAdapter).Capabilities().StickySession {
		eg = &network.EgressGuard{Client: runtime.Gateway.Client}
	}
	executor.EgressCheck = func(ctx context.Context) error {
		if err := c.EnsureFreshNetworkIdentity(ctx, item.AccountID); err != nil {
			return err
		}
		if eg != nil {
			if _, err := eg.Observe(ctx); err != nil {
				return err
			}
		}
		return runtime.CheckMutationGeneration(ctx)
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
	rows, err := tx.QueryContext(ctx, `UPDATE deployments SET lock_version=lock_version+1,state='FAILED',current_step='done',last_error='RESOURCE_DELETED',updated_at=now() WHERE account_id=$1 AND provider_id=$2 AND state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL','REGISTERING_CLIENTS','REGISTERING_TRAFFIC') RETURNING id::text`, accountID, providerID)
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
	if _, err = tx.ExecContext(ctx, `UPDATE accounts a SET deleted_at=now(),runtime_status='DELETED',runtime_status_detail='history retained by soft delete',updated_at=now() WHERE a.id=$1 AND a.deletion_requested_at IS NOT NULL AND a.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED')`, accountID); err != nil {
		return err
	}
	return tx.Commit()
}
