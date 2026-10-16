package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// PublishProviderObservation commits snapshot and state together. A read started
// before a newer account observation or credential change cannot clear its result.
func (c Container) PublishProviderObservation(ctx context.Context, rt AccountRuntime, attempt ProviderAttempt, obs providers.Observation, runtimeStatus, detail string) (bool, error) {
	state := ProviderStateActive
	if strings.EqualFold(strings.TrimSpace(obs.Account.Status), "locked") {
		state = ProviderStateLocked
		runtimeStatus = ProviderStateRuntimeStatus(state)
		body, err := json.Marshal(map[string]any{"provider_state": state, "provider_account_status": obs.Account.Status, "provider_error": obs.Account.StatusMessage, "can_create": false})
		if err != nil {
			return false, err
		}
		detail = string(body)
	}
	raw, err := json.Marshal(obs)
	if err != nil {
		return false, err
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	valid, err := lockProviderAttempt(ctx, tx, attempt)
	if err != nil || !valid || attempt.AccountID != rt.Config.ID || attempt.SecretRef != rt.Config.SecretRef {
		return false, err
	}
	// An older complete snapshot must not suppress authoritative restriction
	// evidence. Publish only the account restriction; keep newer inventory intact.
	if state == ProviderStateLocked {
		var newerComplete bool
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(provider_observation_started_at,provider_state_at,'epoch')>$2 FROM accounts WHERE id=$1`, rt.Config.ID, attempt.Started).Scan(&newerComplete); err != nil {
			return false, err
		}
		if newerComplete {
			result, e := recordProviderRestrictionTx(ctx, tx, attempt, state, true, detail)
			if e != nil {
				return false, e
			}
			n, e := result.RowsAffected()
			if e != nil || n == 0 {
				return false, e
			}
			if e = publishProviderCadence(ctx, tx, attempt); e != nil {
				return false, e
			}
			return true, tx.Commit()
		}
	}
	ready, err := providerObservationNetworkReady(ctx, tx, rt)
	if err != nil {
		return false, err
	}
	// Unknown/warning/trial status is not proof that a hard restriction resolved.
	resolves := strings.EqualFold(strings.TrimSpace(obs.Account.Status), "active") || providers.AccountCondition(rt.Config.Provider, obs.Account, obs.Capacity) == "CAPACITY_FULL"
	if !resolves && runtimeStatus == "READY" {
		runtimeStatus = "PROVIDER_BLOCKED"
	}
	// Preserve legacy lock evidence before a non-resolving observation replaces
	// the latest complete snapshot (the original Digi oc contradiction).
	var retainedLock bool
	if !resolves && state != ProviderStateLocked {
		err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT lower(ps.canonical->'Account'->>'Status')='locked' AND (se.updated_at IS NULL OR se.updated_at<=ps.created_at) FROM provider_snapshots ps LEFT JOIN secrets se ON se.account_id=ps.account_id AND se.id=$2 WHERE ps.account_id=$1 AND ps.canonical IS NOT NULL ORDER BY ps.created_at DESC LIMIT 1),false)`, rt.Config.ID, attempt.SecretRef).Scan(&retainedLock)
		if err != nil {
			return false, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),
 provider_state=CASE WHEN $4='LOCKED' THEN 'LOCKED' WHEN $9 THEN 'ACTIVE' WHEN $12 AND provider_state='ACTIVE' THEN 'LOCKED' ELSE provider_state END,
 provider_state_detail=$5,provider_state_at=clock_timestamp(),provider_observation_started_at=$8,
 provider_restriction_started_at=CASE WHEN $9 OR $4='LOCKED' THEN $8
 WHEN provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN COALESCE(provider_restriction_started_at,provider_state_at)
 WHEN $12 THEN (SELECT created_at FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1)
 ELSE provider_restriction_started_at END,
 provider_success_started_at=CASE WHEN $11 THEN $8 ELSE provider_success_started_at END,
 provider_account_status=$10,provider_account_observed_at=clock_timestamp(),provider_checked_at=clock_timestamp(),
 provider_error_state=CASE WHEN $11 AND COALESCE(provider_error_started_at,'epoch')<=$8 THEN NULL ELSE provider_error_state END,
 provider_error_detail=CASE WHEN $11 AND COALESCE(provider_error_started_at,'epoch')<=$8 THEN NULL ELSE provider_error_detail END,
 runtime_status=CASE WHEN $4='LOCKED' THEN 'PROVIDER_LOCKED'
 WHEN $12 AND provider_state='ACTIVE' THEN 'PROVIDER_LOCKED'
 WHEN NOT $9 AND provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN 'PROVIDER_'||provider_state
 WHEN NOT $11 THEN 'ISOLATION_WAIT'
 WHEN provider_error_started_at>$8 AND COALESCE(provider_error_state,'')<>'' THEN 'PROVIDER_'||provider_error_state ELSE $6 END,
 runtime_status_detail=$5,runtime_status_at=clock_timestamp(),updated_at=clock_timestamp()
 WHERE id=$1 AND secret_ref=$7 AND COALESCE(provider_observation_started_at,provider_state_at,'epoch')<=$8
 AND (NOT ($9 OR $4='LOCKED') OR COALESCE(provider_restriction_started_at,CASE WHEN provider_observation_started_at IS NULL AND provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN provider_state_at END,'epoch')<=$8)`,
		rt.Config.ID, obs.Account.ID, obs.Account.Email, state, detail, runtimeStatus, attempt.SecretRef, attempt.Started, resolves, obs.Account.Status, ready, retainedLock)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	var oldLimit int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT COALESCE((canonical->'Capacity'->>'ComputeLimit')::int,(data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1),0)`, rt.Config.ID).Scan(&oldLimit); err != nil {
		return false, err
	}
	var snapshotID string
	if err = tx.QueryRowContext(ctx, `INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) VALUES(gen_random_uuid(),$1,$2,2,'{}',$3) RETURNING id::text`, rt.Config.ID, rt.Config.Provider, raw).Scan(&snapshotID); err != nil {
		return false, err
	}
	if oldLimit > 0 && obs.Capacity.ComputeLimit > 0 && oldLimit != obs.Capacity.ComputeLimit {
		if _, err = tx.ExecContext(ctx, `INSERT INTO account_capacity_events(account_id,old_limit,new_limit,delta,snapshot_id) VALUES($1,$2,$3,$4,$5)`, rt.Config.ID, oldLimit, obs.Capacity.ComputeLimit, obs.Capacity.ComputeLimit-oldLimit, snapshotID); err != nil {
			return false, err
		}
	}
	if err = publishProviderCadence(ctx, tx, attempt); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
