package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"log"
	"time"
)

// RefreshProviderSnapshots is a bounded one-shot compatibility adapter.
// Production uses RunProviderSnapshots so slow accounts cannot block peers.
func (c Container) RefreshProviderSnapshots(ctx context.Context, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 90 * time.Second
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT id::text FROM accounts WHERE enabled AND deleted_at IS NULL AND deletion_requested_at IS NULL ORDER BY id`)
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
		if ctx.Err() != nil {
			return
		}
		c.refreshProviderAccount(ctx, id, maxAge, 0, 45*time.Second)
	}
}

// Eligibility is evaluated before and under the per-account advisory lease.
func (c Container) providerRefreshDue(ctx context.Context, id string, age, lead time.Duration) (time.Duration, bool) {
	age = c.providerRefreshAge(ctx, id, age)
	var published int
	var state, providerError string
	var checked, attempt time.Time
	err := c.DB.QueryRowContext(ctx, `SELECT provider_inventory_interval_seconds,provider_state,COALESCE(provider_error_state,''),COALESCE(provider_checked_at,'epoch'::timestamptz),GREATEST(COALESCE(provider_refresh_attempt_at,provider_checked_at,'epoch'::timestamptz),COALESCE(provider_error_at,'epoch'::timestamptz),COALESCE(provider_state_at,'epoch'::timestamptz)) FROM accounts WHERE id=$1 AND enabled AND deleted_at IS NULL AND deletion_requested_at IS NULL`, id).Scan(&published, &state, &providerError, &checked, &attempt)
	if err != nil {
		return age, false
	}
	if interval := providerObservationProbeInterval(state, providerError); interval > 0 && time.Since(attempt) < interval {
		return age, false
	}
	threshold := max(time.Second, age-lead)
	var fresh bool
	err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL AND created_at > now()-($2 * interval '1 second'))`, id, threshold.Seconds()).Scan(&fresh)
	if err != nil {
		return age, false
	}
	return age, published != min(300, max(1, int(age/time.Second))) || !providerRefreshMaySkip(fresh, state, providerError, checked, threshold, time.Now().UTC())
}

func (c Container) refreshProviderAccount(parent context.Context, id string, maxAge, lead, deadline time.Duration) {
	// The lease survives the work deadline for bounded failure publication.
	// Shutdown cancels both; no detached goroutine or released live-handler slot.
	outer, cancelOuter := context.WithTimeout(parent, deadline+3*time.Second)
	defer cancelOuter()
	ctx, cancel := context.WithTimeout(outer, deadline)
	defer cancel()
	if _, due := c.providerRefreshDue(ctx, id, maxAge, lead); !due {
		return
	}
	lockTx, err := c.DB.BeginTx(outer, nil)
	if err != nil {
		return
	}
	defer lockTx.Rollback()
	var locked bool
	if err = lockTx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "provider-refresh:"+id).Scan(&locked); err != nil || !locked {
		return
	}
	age, due := c.providerRefreshDue(ctx, id, maxAge, lead)
	if !due {
		return
	}
	intervalSeconds := min(300, max(1, int(age/time.Second)))
	attempt, beginErr := c.BeginProviderAttempt(ctx, id, true)
	if beginErr != nil {
		log.Printf("provider attempt account=%s: %v", id, beginErr)
		return
	}
	attempt.InventoryIntervalSeconds = intervalSeconds
	rt, err := c.Runtime(ctx, id)
	if err != nil {
		providerState := ClassifyAccountProviderError(err, true)
		detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_error": err.Error(), "can_create": false})
		if _, persistErr := c.recordRefreshFailure(outer, attempt, providerState, err, string(detail)); persistErr != nil {
			log.Printf("provider error publication account=%s: %v", id, persistErr)
		}
		return
	}
	if rt.Gateway != nil {
		defer rt.Gateway.CloseIdleConnections()
	}
	obs, err := c.ObserveProvider(ctx, rt)
	if err != nil {
		log.Printf("provider refresh %s: %v", id, err)
		proxyRequired := rt.Config.Network.Mode == "proxy_required"
		providerState := ClassifyAccountProviderError(err, proxyRequired)
		detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_error": err.Error(), "can_create": false})
		if _, persistErr := c.recordRefreshFailure(outer, attempt, providerState, err, string(detail)); persistErr != nil {
			log.Printf("provider error publication account=%s: %v", id, persistErr)
		}
		return
	}
	var capacityWrites []func(context.Context, *sql.Tx) error
	metadata := func(ctx context.Context, tx *sql.Tx) error {
		var err error
		switch rt.Config.Provider {
		case "vultr":
			err = syncVultrResourceRegistryTx(ctx, tx, id, obs.Inventory.Servers)
		case "upcloud":
			err = syncUpCloudResourcesTx(ctx, tx, id, obs.Inventory)
		case "linode":
			err = syncLinodeResourcesTx(ctx, tx, id, obs.Inventory)
		}
		if err != nil {
			return err
		}
		for _, write := range capacityWrites {
			if err = write(ctx, tx); err != nil {
				return err
			}
		}
		return nil
	}
	// Vultr's public API exposes current instances but not the account's
	// Maximum Instances limit. Overlay only API-proven capacity evidence;
	// current usage and lower-bound learning always remain API-derived.
	observedCapacitySource := ""
	if rt.Config.Provider == "vultr" {
		capacityWrites = append(capacityWrites, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO provider_capacity_observations(account_id,compute_limit,source,observed_at,updated_at,lower_bound,probe_in_flight) VALUES($1,0,'vultr_api_lower_bound',now(),now(),$2,false) ON CONFLICT(account_id) DO UPDATE SET lower_bound=GREATEST(provider_capacity_observations.lower_bound,$2),source=CASE WHEN provider_capacity_observations.source IN ('vultr_api_saturation','vultr_api_probe','vultr_api_probe_success') THEN provider_capacity_observations.source ELSE 'vultr_api_lower_bound' END,observed_at=now(),updated_at=now()`, id, obs.Capacity.ComputeInUse)
			return err
		})
		var observedLimit int
		var observedAt time.Time
		if qerr := c.DB.QueryRowContext(ctx, `SELECT compute_limit,observed_at,source FROM provider_capacity_observations WHERE account_id=$1`, id).Scan(&observedLimit, &observedAt, &observedCapacitySource); qerr == nil {
			// A probe may succeed at the provider and then the worker can
			// crash before recording OnCreateSuccess. Fresh inventory
			// above the old exact limit is authoritative proof that the
			// account admitted additional capacity.
			if vultrProbeInventoryProvesSuccess(observedCapacitySource, observedLimit, obs.Capacity.ComputeInUse) {
				observedLimit = obs.Capacity.ComputeInUse
				capacityWrites = append(capacityWrites, func(ctx context.Context, tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, `UPDATE provider_capacity_observations SET compute_limit=$2,lower_bound=GREATEST(lower_bound,$2),source='vultr_api_probe_success',probe_in_flight=false,probe_after=now(),observed_at=now(),updated_at=now() WHERE account_id=$1 AND source='vultr_api_probe'`, id, observedLimit)
					return err
				})
				observedCapacitySource = "vultr_api_probe_success"
			}
			if observedCapacitySource == "vultr_api_saturation" && obs.Capacity.ComputeInUse > observedLimit {
				capacityWrites = append(capacityWrites, func(ctx context.Context, tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, `UPDATE provider_capacity_observations SET compute_limit=0,lower_bound=GREATEST(lower_bound,$2),source='vultr_api_lower_bound',probe_in_flight=false,probe_after=NULL,observed_at=now(),updated_at=now() WHERE account_id=$1 AND source='vultr_api_saturation' AND compute_limit<$2`, id, obs.Capacity.ComputeInUse)
					return err
				})
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
	obs.Account.Condition = providers.AccountCondition(rt.Config.Provider, obs.Account, obs.Capacity)
	detail, _ := json.Marshal(map[string]any{"provider_state": providerState, "provider_account_status": obs.Account.Status, "can_create": canCreate, "droplet_limit": obs.Capacity.ComputeLimit, "provider_droplets": obs.Capacity.ComputeInUse})
	runtimeStatus := "READY"
	probeManagedCapacity := rt.Config.Provider == "vultr" && (observedCapacitySource == "vultr_api_saturation" || observedCapacitySource == "vultr_api_probe_success" || observedCapacitySource == "vultr_api_probe")
	if !canCreate && !probeManagedCapacity {
		runtimeStatus = "PROVIDER_BLOCKED"
	}
	published, publishErr := c.publishProviderObservation(ctx, rt, attempt, obs, runtimeStatus, string(detail), metadata)
	if publishErr != nil {
		log.Printf("provider observation publication %s: %v", id, publishErr)
		return
	}
	if !published {
		return
	}
	c.ReconcileOwnedOrphans(ctx, id)
}

// Failure publication uses the original attempt while the admitted outer lease
// remains live. Pure local deadlines do not claim provider contact or freshness.
func (c Container) recordRefreshFailure(outer context.Context, a ProviderAttempt, state string, cause error, detail string) (bool, error) {
	if outer.Err() != nil {
		return false, outer.Err()
	}
	ctx, cancel := context.WithTimeout(outer, 3*time.Second)
	defer cancel()
	return c.RecordProviderObservation(ctx, a, state, cause, detail)
}
