package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

// Called only while deployment-admission then lifecycle-excess account locks
// are held. A provider ceiling can be lower than Desired; a scheduler deficit
// then is not evidence of a free provider slot. Claim one expired server locally
// before returning to the usual journaled provider deletion path.
func claimCapacityRetirementTx(ctx context.Context, tx *sql.Tx, item droplets.LifecycleItem) (bool, error) {
	var enabled bool
	var state, providerError string
	var desired int
	if err := tx.QueryRowContext(ctx, `SELECT enabled,provider_state,COALESCE(provider_error_state,''),desired_server_count FROM accounts WHERE id=$1 FOR UPDATE`, item.AccountID).Scan(&enabled, &state, &providerError, &desired); err != nil {
		return false, err
	}
	if !enabled || state != "ACTIVE" || providerError != "" || desired <= 0 {
		return false, nil
	}

	if err := lockManagedPopulationTx(ctx, tx, item.AccountID); err != nil {
		return false, err
	}
	eligible, err := lifecycleExpiryEligibleTx(ctx, tx, item)
	if err != nil || !eligible {
		return false, err
	}
	cap, err := capacity.Read(ctx, tx, item.AccountID, 2*time.Minute)
	if errors.Is(err, capacity.ErrSnapshotMissing) || errors.Is(err, capacity.ErrSnapshotStale) || errors.Is(err, capacity.ErrCreateBlocked) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !cap.LimitKnown || cap.Limit <= 0 || cap.InUse < cap.Limit || cap.Pending != 0 || cap.ObservedAt.After(time.Now().Add(30*time.Second)) {
		return false, nil
	}

	var managed, retiring, active, backfill int
	var lastDeleted sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state<>'DELETED' AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=droplets.account_id AND own.provider_resource_id=droplets.provider_resource_id AND own.managed AND own.state<>'deleted')),
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state IN ('RETIRING','DELETING') AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=droplets.account_id AND own.provider_resource_id=droplets.provider_resource_id AND own.managed AND own.state<>'deleted')),
 (SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('PANEL_COMPLETE','READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')),
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state='DELETED' AND backfill_required=true AND replacement_deployment_id IS NULL),
 (SELECT max(updated_at) FROM droplets WHERE account_id=$1 AND state='DELETED')`, item.AccountID).Scan(&managed, &retiring, &active, &backfill, &lastDeleted)
	if err != nil {
		return false, err
	}
	if managed >= desired || retiring != 0 || active != 0 {
		return false, nil
	}
	// Never reuse the full-capacity snapshot from before an earlier deletion.
	if lastDeleted.Valid && !cap.ObservedAt.After(lastDeleted.Time) {
		return false, nil
	}
	// A historical failed backfill can coexist with a fully refilled fleet
	// (for example another admitted deployment filled the provider's last slot).
	// Keep that debt intact. Retire only if fresh native IDs prove every current
	// provider slot is already an owned serving/expired server; counts alone
	// cannot distinguish an unmanaged provider instance from stale local state.
	if backfill != 0 {
		proven, err := backfillAtProvenManagedCeiling(ctx, tx, item.AccountID, managed, lastDeleted)
		if err != nil || !proven {
			return false, err
		}
	}

	var oldest string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM droplets
 WHERE account_id=$1 AND state='EXPIRING' AND replacement_deployment_id IS NULL
 AND expires_at IS NOT NULL AND expires_at<=now() AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=droplets.account_id AND own.provider_resource_id=droplets.provider_resource_id AND own.managed AND own.state<>'deleted')
 ORDER BY expires_at,created_at,id LIMIT 1`, item.AccountID).Scan(&oldest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if oldest != item.ID {
		return false, nil
	}
	res, err := tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now()
 WHERE id=$1 AND account_id=$2 AND state='EXPIRING' AND replacement_deployment_id IS NULL AND expires_at<=now()`, item.ID, item.AccountID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE accounts SET runtime_status='ROTATION_CAPACITY_BREAKING',runtime_status_detail='retiring one expired server at the provider ceiling; scheduler owns backfill',runtime_status_at=now(),updated_at=now() WHERE id=$1`, item.AccountID)
	return err == nil, err
}

// Current row locks, not a stale discovery result, authorize expiry transitions.
// Account-rule OFF keeps each server's recorded lifetime; changing expires_at or
// removing managed ownership revokes an unadmitted expiry transition.
func lifecycleExpiryEligibleTx(ctx context.Context, tx *sql.Tx, item droplets.LifecycleItem) (bool, error) {
	var eligible bool
	err := tx.QueryRowContext(ctx, `SELECT true FROM droplets d JOIN accounts a ON a.id=d.account_id JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id WHERE d.id=$1 AND d.account_id=$2 AND d.state=$3 AND d.provider_resource_id=$4 AND d.expires_at IS NOT NULL AND d.expires_at<=now() AND a.enabled AND r.managed AND r.state<>'deleted' FOR UPDATE OF d FOR SHARE OF a,r`, item.ID, item.AccountID, item.State, item.ProviderID).Scan(&eligible)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return eligible, err
}
func (c Container) transitionExpiredLifecycle(ctx context.Context, item droplets.LifecycleItem, to droplets.State) (bool, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", prefix+item.AccountID); err != nil {
			return false, err
		}
	}
	ok, err := lifecycleExpiryEligibleTx(ctx, tx, item)
	if err != nil || !ok {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE droplets SET state=$3,updated_at=now() WHERE id=$1 AND state=$2 AND expires_at<=now()`, item.ID, item.State, to)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,$3)`, item.AccountID, item.ID, to)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Provider lock is an explicit retirement policy, independent of expiry.
// Its state transition and event share one commit, including owned-resource state.
func (c Container) transitionLockedProviderLifecycle(ctx context.Context, item droplets.LifecycleItem) (bool, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", prefix+item.AccountID); err != nil {
			return false, err
		}
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT true FROM droplets d JOIN accounts a ON a.id=d.account_id JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id WHERE d.id=$1 AND d.account_id=$2 AND d.state=$3 AND d.provider_resource_id=$4 AND a.provider_state='LOCKED' AND r.managed AND r.state<>'deleted' FOR UPDATE OF d FOR SHARE OF a,r`, item.ID, item.AccountID, item.State, item.ProviderID).Scan(&eligible)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE resources SET state='retiring',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND managed AND state<>'deleted'`, item.AccountID, item.ProviderID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	res, err = tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND state=$2 AND provider_resource_id=$3`, item.ID, item.State, item.ProviderID)
	if err != nil {
		return false, err
	}
	n, err = res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}

	if _, err = tx.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')`, item.AccountID, item.ID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Freeze revocation of the currently counted owned population until the
// account-scoped admission transaction commits. New ownership can only make
// this conservative count larger; absent/unowned rows cannot authorize deletion.
func lockManagedPopulationTx(ctx context.Context, tx *sql.Tx, account string) error {
	rows, err := tx.QueryContext(ctx, `SELECT provider_resource_id FROM resources WHERE account_id=$1 AND managed AND state<>'deleted' ORDER BY provider_resource_id FOR SHARE`, account)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

func backfillAtProvenManagedCeiling(ctx context.Context, tx *sql.Tx, accountID string, managed int, lastDeleted sql.NullTime) (bool, error) {
	var raw []byte
	var candidates int
	var snapshotAt time.Time
	// This exception derives every capacity/inventory fact from one selected
	// row in one statement. Earlier admission reads are only conservative
	// guards: a concurrent refresh/update cannot mix their fields into proof.
	// Equal newest timestamps are ambiguous and remain blocked.
	err := tx.QueryRowContext(ctx, `SELECT canonical,created_at,count(*) OVER(PARTITION BY created_at) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, accountID).Scan(&raw, &snapshotAt, &candidates)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if candidates != 1 {
		return false, nil
	}
	var observation struct {
		Inventory struct {
			Servers    []struct{ ID string }
			ObservedAt time.Time
		}
		Capacity struct {
			ObservedAt                 time.Time
			ComputeLimit, ComputeInUse int
			LimitKnown                 bool
		}
	}
	if json.Unmarshal(raw, &observation) != nil {
		return false, nil
	}
	inventory := observation.Inventory
	capacityAt := observation.Capacity.ObservedAt
	nativeCap := observation.Capacity
	if !nativeCap.LimitKnown || nativeCap.ComputeLimit <= 0 || nativeCap.ComputeInUse != managed || nativeCap.ComputeLimit != managed || time.Since(snapshotAt) > 2*time.Minute || snapshotAt.After(time.Now().Add(30*time.Second)) || (lastDeleted.Valid && !snapshotAt.After(lastDeleted.Time)) {
		return false, nil
	}
	if len(inventory.Servers) != managed || inventory.ObservedAt.IsZero() || capacityAt.IsZero() {
		return false, nil
	}
	// Debt retirement requires a tightly assembled observation: inventory and
	// capacity sampling at most one second apart, persisted within 30 seconds.
	// Slower collection remains usable elsewhere but cannot authorize this
	// exceptional retirement. Native timestamps also prevent copied capacity
	// updates from rejuvenating an older inventory.
	skew := capacityAt.Sub(inventory.ObservedAt)
	inventoryLag := snapshotAt.Sub(inventory.ObservedAt)
	capacityLag := snapshotAt.Sub(capacityAt)
	if skew < -time.Second || skew > time.Second || inventoryLag < -time.Second || inventoryLag > 30*time.Second || capacityLag < -time.Second || capacityLag > 30*time.Second {
		return false, nil
	}
	now := time.Now()
	if now.Sub(inventory.ObservedAt) > 2*time.Minute || inventory.ObservedAt.After(now.Add(30*time.Second)) || (lastDeleted.Valid && !inventory.ObservedAt.After(lastDeleted.Time)) {
		return false, nil
	}
	ids := make([]string, 0, len(inventory.Servers))
	seen := make(map[string]bool, len(inventory.Servers))
	for _, server := range inventory.Servers {
		if server.ID == "" || seen[server.ID] {
			return false, nil
		}
		seen[server.ID] = true
		ids = append(ids, server.ID)
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return false, err
	}
	var matched int
	var uncertain bool
	err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT d.provider_resource_id),
 EXISTS(SELECT 1 FROM operations o WHERE o.account_id=$1 AND o.kind='CREATE_DROPLET' AND o.state IN ('planned','running','verifying','unknown'))
 FROM droplets d WHERE d.account_id=$1 AND d.state IN ('READY','EXPIRING')
 AND d.provider_resource_id IN (SELECT jsonb_array_elements_text($2::jsonb))
 AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=d.account_id AND own.provider_resource_id=d.provider_resource_id AND own.managed AND own.state<>'deleted')`, accountID, string(encoded)).Scan(&matched, &uncertain)
	return err == nil && !uncertain && matched == managed, err
}
