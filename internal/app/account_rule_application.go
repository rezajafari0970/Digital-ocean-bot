package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
)

// AccountBuildRulesTx excludes desired and cadence: changing population or timing
// alone must never replace every server. JSONB equality preserves duplicate choices.
func AccountBuildRulesTx(ctx context.Context, tx *sql.Tx, id string) (string, int, error) {
	var spec string
	var desired int
	err := tx.QueryRowContext(ctx, `SELECT jsonb_build_object(
 'regions',preferred_regions,'region',preferred_region,'sizes',preferred_sizes,
 'images',preferred_images,'image',preferred_image,
 'lifetime_min',server_lifetime_min_seconds,'lifetime_max',server_lifetime_max_seconds,
 'fallback',fallback_any_region)::text,desired_server_count FROM accounts WHERE id=$1`, id).Scan(&spec, &desired)
	return spec, desired, err
}

// SaveAccountRuleApplicationTx shares the caller's deployment-admission lock.
// It records intent only; Save never performs a provider mutation.
func SaveAccountRuleApplicationTx(ctx context.Context, tx *sql.Tx, id, before string, oldDesired int, apply bool) error {
	after, desired, err := AccountBuildRulesTx(ctx, tx, id)
	if err != nil {
		return err
	}
	changed := before != after
	_, err = tx.ExecContext(ctx, `INSERT INTO account_rule_application(account_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_rule_application SET
 apply_to_existing=$2,
 revision=revision+CASE WHEN $3 THEN 1 ELSE 0 END,
 replacement_revision=CASE WHEN NOT $2 THEN NULL WHEN $3 THEN revision+1 ELSE replacement_revision END,
 shrink_pending=CASE WHEN NOT $2 THEN false WHEN $4 THEN true WHEN $5 THEN false ELSE shrink_pending END,
 next_action_at=CASE WHEN ($2 AND $3) OR ($2 AND $4) THEN now() ELSE next_action_at END,
 status=CASE WHEN NOT $2 THEN 'FUTURE_ONLY' WHEN $3 OR $4 THEN 'QUEUED' ELSE status END,
 detail=CASE WHEN NOT $2 THEN 'Existing servers keep their recorded lifetime; new builds use current rules.'
 WHEN $3 THEN 'Replace older build revisions at the configured spacing.'
 WHEN $4 THEN 'Retire only excess servers, earliest expiry first.' ELSE detail END,
 updated_at=now() WHERE account_id=$1`, id, apply, changed, desired < oldDesired, desired > oldDesired)
	if err != nil {
		return err
	}
	// Save and lifecycle admission share the same lock order. OFF cancels any
	// locally queued retirement that has not crossed the durable start boundary.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "lifecycle-excess:"+id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `WITH cancelled AS (
 UPDATE droplets d SET state=p.waiting_previous_state,updated_at=now()
 FROM account_rule_application p JOIN accounts a ON a.id=p.account_id
 WHERE p.account_id=$1 AND d.id=p.waiting_droplet_id AND d.state='RETIRING'
 AND NOT p.retirement_started AND a.deletion_requested_at IS NULL
 AND (NOT p.apply_to_existing OR
 (SELECT count(*) FROM droplets live WHERE live.account_id=$1 AND live.state<>'DELETED' AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=live.account_id AND own.provider_resource_id=live.provider_resource_id AND own.managed AND own.state<>'deleted')) < a.desired_server_count OR
 (p.waiting_reason='SHRINK' AND (SELECT count(*) FROM droplets live WHERE live.account_id=$1 AND live.state<>'DELETED' AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=live.account_id AND own.provider_resource_id=live.provider_resource_id AND own.managed AND own.state<>'deleted')) <= a.desired_server_count))
 RETURNING d.id
 ) UPDATE account_rule_application SET waiting_droplet_id=NULL,waiting_reason='',retirement_started=false
 WHERE account_id=$1 AND waiting_droplet_id IN (SELECT id FROM cancelled)`, id); err != nil {
		return err
	}
	if desired > oldDesired {
		_, err = tx.ExecContext(ctx, `UPDATE schedules SET next_run_at=now() WHERE account_id=$1 AND enabled`, id)
	}
	return err
}

// ProcessAccountRuleApplications only claims an owned server for the ordinary
// lifecycle executor. Existing provider deletion/reconciliation remains authoritative.
func (c Container) ProcessAccountRuleApplications(ctx context.Context) error {
	rows, err := c.DB.QueryContext(ctx, `SELECT p.account_id::text FROM account_rule_application p
 JOIN accounts a ON a.id=p.account_id
 WHERE p.apply_to_existing AND (p.replacement_revision IS NOT NULL OR p.shrink_pending OR p.waiting_droplet_id IS NOT NULL)
 AND a.enabled AND a.deletion_requested_at IS NULL
 ORDER BY p.updated_at,p.account_id LIMIT 100`)
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
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		stepCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = c.ProcessAccountRuleApplication(stepCtx, id)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("account rule application %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// ProcessAccountRuleApplication uses no remote calls while holding SQL locks.
func (c Container) ProcessAccountRuleApplication(ctx context.Context, id string) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		var acquired bool
		if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, prefix+id).Scan(&acquired); err != nil {
			return err
		}
		if !acquired {
			return nil
		}
	}
	var apply, shrink, enabled, deleting bool
	var revision sql.NullInt64
	var next sql.NullTime
	var waiting, runtimeStatus, providerState, providerError string
	var desired, spacingMin, spacingMax int
	err = tx.QueryRowContext(ctx, `SELECT p.apply_to_existing,p.replacement_revision,p.shrink_pending,
 p.next_action_at,COALESCE(p.waiting_droplet_id::text,''),a.enabled,a.deletion_requested_at IS NOT NULL,
 a.desired_server_count,a.build_spacing_minutes,a.build_spacing_max_minutes,a.runtime_status,
 a.provider_state,COALESCE(a.provider_error_state,'')
 FROM account_rule_application p JOIN accounts a ON a.id=p.account_id WHERE p.account_id=$1
 FOR UPDATE OF p`, id).Scan(&apply, &revision, &shrink, &next, &waiting, &enabled, &deleting, &desired, &spacingMin, &spacingMax, &runtimeStatus, &providerState, &providerError)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if !apply || !enabled || deleting {
		return nil
	}
	finish := func(status, detail string) error {
		if _, e := tx.ExecContext(ctx, `UPDATE account_rule_application SET status=$2,detail=$3,updated_at=now() WHERE account_id=$1`, id, status, detail); e != nil {
			return e
		}
		return tx.Commit()
	}
	if err = lockManagedPopulationTx(ctx, tx, id); err != nil {
		return err
	}
	var managed, ready, retiring, pending, old int
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state<>'DELETED' AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=droplets.account_id AND own.provider_resource_id=droplets.provider_resource_id AND own.managed AND own.state<>'deleted')),
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state='READY' AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=droplets.account_id AND own.provider_resource_id=droplets.provider_resource_id AND own.managed AND own.state<>'deleted')),
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state IN ('RETIRING','DELETING') AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=droplets.account_id AND own.provider_resource_id=droplets.provider_resource_id AND own.managed AND own.state<>'deleted')),
 (SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') AND (droplet_id IS NULL OR EXISTS(SELECT 1 FROM droplets WHERE id=deployments.droplet_id AND state<>'DELETED'))),
 (SELECT count(*) FROM droplets d WHERE d.account_id=$1 AND d.state<>'DELETED' AND EXISTS(SELECT 1 FROM resources own WHERE own.account_id=d.account_id AND own.provider_resource_id=d.provider_resource_id AND own.managed AND own.state<>'deleted')
 AND COALESCE((SELECT max(build_rules_revision) FROM deployments WHERE droplet_id=d.id),0)<$2)
 + (SELECT count(*) FROM deployments WHERE account_id=$1 AND droplet_id IS NULL AND build_rules_revision<$2
 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE'))`, id, revision.Int64).Scan(&managed, &ready, &retiring, &pending, &old)
	if err != nil {
		return err
	}
	if waiting != "" {
		var state string
		if err = tx.QueryRowContext(ctx, `SELECT state FROM droplets WHERE id=$1 AND account_id=$2`, waiting, id).Scan(&state); err != nil {
			return err
		}
		if state != "DELETED" {
			return finish("WAITING_DELETION", "Waiting for confirmed deletion of the previously selected server.")
		}
		if ready < desired {
			return finish("WAITING_REPLACEMENT", "Waiting for the freed slot to recover to the desired ready-server count.")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE account_rule_application SET waiting_droplet_id=NULL WHERE account_id=$1`, id); err != nil {
			return err
		}
	}
	if shrink && managed <= desired && pending == 0 {
		shrink = false
		if _, err = tx.ExecContext(ctx, `UPDATE account_rule_application SET shrink_pending=false WHERE account_id=$1`, id); err != nil {
			return err
		}
	}
	if revision.Valid && old == 0 {
		revision.Valid = false
		if _, err = tx.ExecContext(ctx, `UPDATE account_rule_application SET replacement_revision=NULL WHERE account_id=$1`, id); err != nil {
			return err
		}
	}
	if !shrink && !revision.Valid {
		return finish("COMPLETE", "Current requested changes have completed.")
	}
	if next.Valid && time.Now().Before(next.Time) {
		return finish("WAITING_SPACING", "Waiting for the next configured build-spacing window.")
	}
	if retiring > 0 || pending > 0 {
		return finish("WAITING_WORK", "Waiting for current build or deletion work to settle.")
	}
	if runtimeStatus != "READY" || providerState != "ACTIVE" || providerError != "" {
		return finish("BLOCKED", "Account/provider is not ready for automatic rule application.")
	}
	var blocked, proxyOK bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_create_blocks WHERE account_id=$1),
 EXISTS(SELECT 1 FROM network_profiles n WHERE n.account_id=$1 AND (n.mode='direct' OR (n.mode='proxy_required' AND EXISTS(SELECT 1 FROM proxies p WHERE p.id=n.proxy_id AND p.status='healthy'))))`, id).Scan(&blocked, &proxyOK)
	if err != nil {
		return err
	}
	shrinkNow := shrink && managed > desired
	if !proxyOK {
		return finish("BLOCKED", "Account connection is not healthy.")
	}
	if !shrinkNow {
		if blocked {
			return finish("BLOCKED", "Provider creation is blocked; existing servers retained.")
		}
		cap, capErr := capacity.Read(ctx, tx, id, 2*time.Minute)
		if capErr != nil {
			return finish("BLOCKED", "Fresh provider capacity is required before replacement.")
		}
		if cap.Pending > 0 || (cap.LimitKnown && (cap.Limit < 1 || cap.InUse > cap.Limit)) {
			return finish("BLOCKED", "Resolve pending provider work or capacity restrictions before replacement.")
		}
		if ready < desired {
			return finish("WAITING_CAPACITY", "Fill the desired ready-server count before replacing another server.")
		}
	}
	var candidate, previousState string
	err = tx.QueryRowContext(ctx, `SELECT d.id::text,d.state FROM droplets d JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id AND r.managed AND r.state<>'deleted'
 WHERE d.account_id=$1 AND (d.state='READY' OR ($2 AND d.state='EXPIRING'))
 AND (NOT $2 OR d.state='EXPIRING' OR $4 > (SELECT desired_server_count FROM accounts WHERE id=$1)) AND COALESCE(d.provider_resource_id,'')<>''
 AND d.replacement_deployment_id IS NULL
 AND ($2 OR COALESCE((SELECT max(build_rules_revision) FROM deployments WHERE droplet_id=d.id),0)<$3)
 ORDER BY d.expires_at NULLS LAST,d.created_at,d.id LIMIT 1 FOR UPDATE OF d FOR SHARE OF r`, id, shrinkNow, revision.Int64, ready).Scan(&candidate, &previousState)
	if err == sql.ErrNoRows {
		return finish("WAITING_WORK", "Waiting for an eligible owned server.")
	}
	if err != nil {
		return err
	}
	// Never lower healthy serving capacity for growth, and shrink only exact excess.
	if !shrinkNow && (ready < desired || !revision.Valid) {
		return finish("WAITING_CAPACITY", "No server can be retired for this request yet.")
	}
	res, err := tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND account_id=$2 AND state=$3 AND EXISTS(SELECT 1 FROM resources r WHERE r.account_id=droplets.account_id AND r.provider_resource_id=droplets.provider_resource_id AND r.managed AND r.state<>'deleted')`, candidate, id, previousState)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')`, id, candidate); err != nil {
		return err
	}
	if spacingMin < 1 {
		spacingMin = 1
	}
	if spacingMax < spacingMin {
		spacingMax = spacingMin
	}
	minutes := spacingMin
	if spacingMax > spacingMin {
		minutes += rand.Intn(spacingMax - spacingMin + 1)
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_rule_application SET waiting_droplet_id=$2,
 next_action_at=now()+make_interval(mins=>$3),status='WAITING_DELETION',
 retirement_started=false,waiting_previous_state=$5,waiting_reason=$6,
 detail=$4,updated_at=now() WHERE account_id=$1`, id, candidate, minutes, func() string {
		if shrinkNow {
			return "Retiring one excess server; desired-only changes do not rotate other servers."
		}
		return "Replacing one old build revision; the next retirement waits for recovery."
	}(), previousState, func() string {
		if shrinkNow {
			return "SHRINK"
		}
		return "REPLACE"
	}())
	if err != nil {
		return err
	}
	return tx.Commit()
}

// AdmitAccountRuleRetirement is the durable start boundary. A stale lifecycle
// item cannot delete a server restored by a concurrent OFF Save. Once admitted,
// ordinary provider outcome reconciliation must complete even if OFF is saved.
func (c Container) AdmitAccountRuleRetirement(ctx context.Context, accountID, dropletID string) (bool, error) {
	return c.admitRetirement(ctx, accountID, dropletID, "")
}

// Lifecycle callers must bind authorization to the exact provider deletion target.
func (c Container) AdmitLifecycleRetirement(ctx context.Context, accountID, dropletID, providerID string) (bool, error) {
	if providerID == "" {
		return false, nil
	}
	return c.admitRetirement(ctx, accountID, dropletID, providerID)
}
func (c Container) admitRetirement(ctx context.Context, accountID, dropletID, providerID string) (bool, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+accountID); err != nil {
			return false, err
		}
	}
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT d.state FROM droplets d JOIN accounts a ON a.id=d.account_id JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id WHERE a.provider_state NOT IN ('LOCKED','BILLING_BLOCKED') AND d.id=$1 AND d.account_id=$2 AND ($3='' OR d.provider_resource_id=$3) AND r.managed AND r.state<>'deleted' FOR UPDATE OF d FOR SHARE OF r`, dropletID, accountID, providerID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if state != "RETIRING" {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE account_rule_application SET retirement_started=true,updated_at=now()
 WHERE account_id=$1 AND waiting_droplet_id=$2`, accountID, dropletID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
