package app

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"log"
	"time"
)

// RefreshProviderSnapshots keeps capacity state fresh with one coordinated
// discovery per account. PostgreSQL advisory locks collapse concurrent workers.
func (c Container) RefreshProviderSnapshots(ctx context.Context, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 90 * time.Second
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT id::text FROM accounts WHERE enabled=true AND deleted_at IS NULL`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		maxAge := c.providerRefreshAge(ctx, id, maxAge)
		var providerState, providerError string
		var providerCheckedAt time.Time
		_ = c.DB.QueryRowContext(ctx, `SELECT provider_state,COALESCE(provider_error_state,''),COALESCE(provider_checked_at,'epoch'::timestamptz) FROM accounts WHERE id=$1`, id).Scan(&providerState, &providerError, &providerCheckedAt)
		effectiveState := providerState
		if providerError != "" {
			effectiveState = providerError
		}
		if interval := ProviderProbeInterval(effectiveState); interval > 0 && time.Since(providerCheckedAt) < interval {
			continue
		}
		var fresh bool
		_ = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL AND created_at > now()-($2 * interval '1 second'))`, id, int(maxAge/time.Second)).Scan(&fresh)
		if providerRefreshMaySkip(fresh, providerState, providerError, providerCheckedAt, maxAge, time.Now().UTC()) {
			continue
		}
		lockTx, lockErr := c.DB.BeginTx(ctx, nil)
		if lockErr != nil {
			continue
		}
		var locked bool
		if err := lockTx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "provider-refresh:"+id).Scan(&locked); err != nil || !locked {
			_ = lockTx.Rollback()
			continue
		}
		func() {
			defer lockTx.Rollback()
			// Re-check account state and freshness after acquiring the lock: another
			// coordinator may have refreshed or changed provider state while we waited.
			if qerr := c.DB.QueryRowContext(ctx, `SELECT provider_state,COALESCE(provider_error_state,''),COALESCE(provider_checked_at,'epoch'::timestamptz) FROM accounts WHERE id=$1`, id).Scan(&providerState, &providerError, &providerCheckedAt); qerr != nil {
				return
			}
			effectiveState = providerState
			if providerError != "" {
				effectiveState = providerError
			}
			if interval := ProviderProbeInterval(effectiveState); interval > 0 && time.Since(providerCheckedAt) < interval {
				return
			}
			_ = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL AND created_at > now()-($2 * interval '1 second'))`, id, int(maxAge/time.Second)).Scan(&fresh)
			if providerRefreshMaySkip(fresh, providerState, providerError, providerCheckedAt, maxAge, time.Now().UTC()) {
				return
			}
			rt, err := c.Runtime(ctx, id)
			if err != nil {
				providerState := ClassifyAccountProviderError(err, true)
				detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_error": err.Error(), "can_create": false})
				c.RecordProviderObservation(ctx, id, providerState, err, string(detail))
				return
			}
			obs, err := c.ObserveProvider(ctx, rt)
			if rt.Gateway != nil {
				rt.Gateway.CloseIdleConnections()
			}
			if err != nil {
				log.Printf("provider refresh %s: %v", id, err)
				proxyRequired := rt.Config.Network.Mode == "proxy_required"
				providerState := ClassifyAccountProviderError(err, proxyRequired)
				detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_error": err.Error(), "can_create": false})
				c.RecordProviderObservation(ctx, id, providerState, err, string(detail))
				return
			}
			if rt.Config.Provider == "vultr" {
				if syncErr := c.syncVultrResourceRegistry(ctx, id, obs.Inventory.Servers); syncErr != nil {
					log.Printf("vultr resource registry sync account=%s: %v", id, syncErr)
				}
			}
			if rt.Config.Provider == "upcloud" {
				if err := c.SyncUpCloudResources(ctx, id, obs.Inventory); err != nil {
					log.Printf("upcloud inventory persistence account=%s: %v", id, err)
					return
				}
			}
			// Vultr's public API exposes current instances but not the account's
			// Maximum Instances limit. Overlay only API-proven capacity evidence;
			// current usage and lower-bound learning always remain API-derived.
			observedCapacitySource := ""
			if rt.Config.Provider == "vultr" {
				_, _ = c.DB.ExecContext(ctx, `INSERT INTO provider_capacity_observations(account_id,compute_limit,source,observed_at,updated_at,lower_bound,probe_in_flight) VALUES($1,0,'vultr_api_lower_bound',now(),now(),$2,false) ON CONFLICT(account_id) DO UPDATE SET lower_bound=GREATEST(provider_capacity_observations.lower_bound,$2),source=CASE WHEN provider_capacity_observations.source IN ('vultr_api_saturation','vultr_api_probe','vultr_api_probe_success') THEN provider_capacity_observations.source ELSE 'vultr_api_lower_bound' END,observed_at=now(),updated_at=now()`, id, obs.Capacity.ComputeInUse)
				var observedLimit int
				var observedAt time.Time
				if qerr := c.DB.QueryRowContext(ctx, `SELECT compute_limit,observed_at,source FROM provider_capacity_observations WHERE account_id=$1`, id).Scan(&observedLimit, &observedAt, &observedCapacitySource); qerr == nil {
					// A probe may succeed at the provider and then the worker can
					// crash before recording OnCreateSuccess. Fresh inventory
					// above the old exact limit is authoritative proof that the
					// account admitted additional capacity.
					if vultrProbeInventoryProvesSuccess(observedCapacitySource, observedLimit, obs.Capacity.ComputeInUse) {
						observedLimit = obs.Capacity.ComputeInUse
						_, _ = c.DB.ExecContext(ctx, `UPDATE provider_capacity_observations SET compute_limit=$2,lower_bound=GREATEST(lower_bound,$2),source='vultr_api_probe_success',probe_in_flight=false,probe_after=now(),observed_at=now(),updated_at=now() WHERE account_id=$1 AND source='vultr_api_probe'`, id, observedLimit)
						observedCapacitySource = "vultr_api_probe_success"
					}
					if observedCapacitySource == "vultr_api_saturation" && obs.Capacity.ComputeInUse > observedLimit {
						_, _ = c.DB.ExecContext(ctx, `UPDATE provider_capacity_observations SET compute_limit=0,lower_bound=GREATEST(lower_bound,$2),source='vultr_api_lower_bound',probe_in_flight=false,probe_after=NULL,observed_at=now(),updated_at=now() WHERE account_id=$1 AND source='vultr_api_saturation' AND compute_limit<$2`, id, obs.Capacity.ComputeInUse)
						observedCapacitySource = "vultr_api_lower_bound"
						observedLimit = 0
					}
					if observedCapacitySource != "vultr_api_lower_bound" {
						obs.Capacity.ComputeLimit = observedLimit
						obs.Capacity.LimitKnown = true
					}
					if observedAt.After(obs.Capacity.ObservedAt) {
						obs.Capacity.ObservedAt = observedAt
					}
				}
			}
			providerState := "active"
			accountAllowed, statusErr := capacity.AccountStatusAllowsCreate(ctx, c.DB, id, obs.Account.Status)
			if statusErr != nil {
				return
			}
			canCreate := accountAllowed && providers.CanCreateCapacity(obs.Capacity)
			if !accountAllowed {
				providerState = "disabled"
			} else if !providers.CanCreateCapacity(obs.Capacity) {
				providerState = "cannot_create"
			}
			canonical, _ := json.Marshal(obs)
			var oldLimit int
			_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE((canonical->'Capacity'->>'ComputeLimit')::int,(data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&oldLimit)
			var snapshotID string
			if err = c.DB.QueryRowContext(ctx, `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,$2,2,$3,$4) RETURNING id::text`, id, rt.Config.Provider, []byte(`{}`), canonical).Scan(&snapshotID); err != nil {
				return
			}
			newLimit := obs.Capacity.ComputeLimit
			if oldLimit > 0 && newLimit > 0 && oldLimit != newLimit {
				_, _ = c.DB.ExecContext(ctx, `INSERT INTO account_capacity_events(account_id,old_limit,new_limit,delta,snapshot_id) VALUES($1,$2,$3,$4,$5)`, id, oldLimit, newLimit, newLimit-oldLimit, snapshotID)
				log.Printf("capacity change %s: %d -> %d (%+d)", id, oldLimit, newLimit, newLimit-oldLimit)
			}
			detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_account_status": obs.Account.Status, "can_create": canCreate, "droplet_limit": obs.Capacity.ComputeLimit, "provider_droplets": obs.Capacity.ComputeInUse})
			runtimeStatus := "READY"
			probeManagedCapacity := rt.Config.Provider == "vultr" && (observedCapacitySource == "vultr_api_saturation" || observedCapacitySource == "vultr_api_probe_success" || observedCapacitySource == "vultr_api_probe")
			if !canCreate && !probeManagedCapacity {
				runtimeStatus = "PROVIDER_BLOCKED"
			}
			_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),provider_state='ACTIVE',provider_state_detail=$5,provider_state_at=now(),provider_error_state=NULL,provider_error_detail=NULL,provider_checked_at=now(),runtime_status=$4,runtime_status_detail=$5,runtime_status_at=now(),updated_at=now() WHERE id=$1`, id, obs.Account.ID, obs.Account.Email, runtimeStatus, string(detail))
			c.ReconcileOwnedOrphans(ctx, id)
		}()
	}
}

func (c Container) RecordProviderObservation(ctx context.Context, id, state string, err error, detail string) {
	if IsProviderObservationError(state) {
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET provider_error_state=$2,provider_error_detail=$3,provider_checked_at=now(),runtime_status=$4,runtime_status_detail=$3,runtime_status_at=now(),next_build_at=NULL,updated_at=now() WHERE id=$1`, id, state, detail, ProviderStateRuntimeStatus(state))
		return
	}
	_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET provider_state=$2,provider_state_detail=$3,provider_state_at=now(),provider_error_state=NULL,provider_error_detail=NULL,provider_checked_at=now(),runtime_status=$4,runtime_status_detail=$3,runtime_status_at=now(),next_build_at=NULL,updated_at=now() WHERE id=$1`, id, state, detail, ProviderStateRuntimeStatus(state))
}
