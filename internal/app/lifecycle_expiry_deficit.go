package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// The caller holds deployment-admission then lifecycle-excess. This first
// rollout is deliberately Linode-only: usage-only capacity must not make
// Desired a prerequisite for retiring an expired server.
func claimExpiredDeficitRetirementTx(ctx context.Context, tx *sql.Tx, item droplets.LifecycleItem) (string, error) {
	ok, err := expiryDeficitProofTx(ctx, tx, item, false)
	if err != nil || !ok {
		return "", err
	}
	var oldest string
	err = tx.QueryRowContext(ctx, `SELECT d.id::text FROM droplets d
 WHERE d.account_id=$1 AND d.state='EXPIRING' AND d.replacement_deployment_id IS NULL AND d.expires_at<=now()
 AND EXISTS(SELECT 1 FROM resources r WHERE r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id AND r.managed AND r.state<>'deleted')
 ORDER BY d.expires_at,d.created_at,d.id LIMIT 1`, item.AccountID).Scan(&oldest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil || oldest != item.ID {
		return "", err
	}
	var claim string
	err = tx.QueryRowContext(ctx, `INSERT INTO lifecycle_deficit_claims(droplet_id,account_id,provider_resource_id)
 VALUES($1,$2,$3) RETURNING claim_id::text`, item.ID, item.AccountID, item.ProviderID).Scan(&claim)
	if err != nil {
		return "", err
	}
	res, err := tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND account_id=$2 AND provider_resource_id=$3 AND state='EXPIRING' AND replacement_deployment_id IS NULL AND expires_at<=now()`, item.ID, item.AccountID, item.ProviderID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", errors.New("expiry deficit claim lost target")
	}
	return claim, nil
}

// Each proof is local and short. Locking the parent account FOR UPDATE also
// fences inserts into its FK-backed operations/deployments/provider snapshots;
// existing active work is rejected before observation rows are selected.
func expiryDeficitProofTx(ctx context.Context, tx *sql.Tx, item droplets.LifecycleItem, claimed bool) (bool, error) {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT provider='linode' AND enabled AND provider_state='ACTIVE'
 AND COALESCE(provider_error_state,'')='' AND runtime_status='READY' AND desired_server_count>0
 FROM accounts WHERE id=$1 FOR UPDATE`, item.AccountID).Scan(&allowed)
	if err != nil || !allowed {
		return false, err
	}
	if err = lockManagedPopulationTx(ctx, tx, item.AccountID); err != nil {
		return false, err
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT true FROM droplets d JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id
 WHERE d.id=$1 AND d.account_id=$2 AND d.provider_resource_id=$3 AND d.state=$4
 AND d.expires_at<=now() AND d.replacement_deployment_id IS NULL AND r.managed AND r.state<>'deleted'
 FOR UPDATE OF d FOR SHARE OF r`, item.ID, item.AccountID, item.ProviderID, item.State).Scan(&eligible)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var blocked bool
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM droplets d WHERE d.account_id=$1 AND d.state<>'DELETED'
 AND EXISTS(SELECT 1 FROM resources r WHERE r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id AND r.managed AND r.state<>'deleted')) >= (SELECT desired_server_count FROM accounts WHERE id=$1)
 OR EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND state IN ('RETIRING','DELETING') AND (NOT $3 OR id<>$2))
 OR EXISTS(SELECT 1 FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','PANEL_COMPLETE','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK') AND (droplet_id IS NULL OR EXISTS(SELECT 1 FROM droplets d WHERE d.id=deployments.droplet_id AND d.state<>'DELETED')))
 OR EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND kind IN ('CREATE_DROPLET','DELETE_DROPLET') AND state IN ('planned','running','unknown','verifying'))
 OR EXISTS(SELECT 1 FROM account_create_blocks WHERE account_id=$1)
 OR EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND idempotency_key='lifecycle-delete:'||$2::text)
 OR EXISTS(SELECT 1 FROM droplets d WHERE d.account_id=$1 AND d.state='DELETED' AND d.backfill_required AND d.replacement_deployment_id IS NULL)
 OR EXISTS(SELECT 1 FROM lifecycle_deficit_claims cl JOIN droplets d ON d.id=cl.droplet_id
 WHERE cl.account_id=$1 AND cl.recovered_at IS NULL AND d.state='DELETED' AND NOT EXISTS(SELECT 1 FROM deployments dep JOIN droplets n ON n.id=dep.droplet_id
 WHERE dep.id=d.replacement_deployment_id AND dep.account_id=$1 AND n.account_id=$1 AND dep.state='PANEL_COMPLETE' AND n.state='READY'))`, item.AccountID, item.ID, claimed).Scan(&blocked)
	if err != nil || blocked {
		return false, err
	}
	// There must already be a configured, enabled route for normal backfill.
	var schedule string
	err = tx.QueryRowContext(ctx, `SELECT s.id::text FROM schedules s
 JOIN deployment_profiles p ON p.id=s.profile_id AND p.account_id=s.account_id
 JOIN installers i ON i.name=p.config->'installer_ref'->>'name' AND i.version::text=p.config->'installer_ref'->>'version'
 JOIN xui_database_templates t ON t.id::text=p.config->>'database_template_id'
 WHERE s.account_id=$1 AND s.enabled AND p.enabled AND i.active AND i.manifest @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb
 AND t.active AND t.storage_path NOT IN ('','pending') ORDER BY s.id LIMIT 1 FOR SHARE OF s,p,i,t`, item.AccountID).Scan(&schedule)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return expiryDeficitServingProofTx(ctx, tx, item, true)
}

// This same proof records backfill recovery during ordinary reconciliation,
// so a replacement that later expires cannot reopen a completed obligation.
func expiryDeficitServingProofTx(ctx context.Context, tx *sql.Tx, item droplets.LifecycleItem, requireOld bool) (bool, error) {
	var err error
	var raw []byte
	var at time.Time
	var count int
	var provider string
	err = tx.QueryRowContext(ctx, `SELECT canonical,created_at,count(*) OVER(PARTITION BY created_at),provider
 FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, item.AccountID).Scan(&raw, &at, &count, &provider)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var observation providers.Observation
	now := time.Now()
	if count != 1 || provider != "linode" || json.Unmarshal(raw, &observation) != nil || !recentExpiryProof(at, now, 2*time.Minute) || !recentExpiryProof(observation.Inventory.ObservedAt, now, 2*time.Minute) || observation.Account.Status != "active" {
		return false, nil
	}
	lag := at.Sub(observation.Inventory.ObservedAt)
	if lag < 0 || lag > 30*time.Second {
		return false, nil
	}
	seen := map[string]bool{}
	ready := []string{}
	for _, server := range observation.Inventory.Servers {
		if server.ID == "" || seen[server.ID] {
			return false, nil
		}
		seen[server.ID] = true
		if server.ID != item.ProviderID && server.Ready && server.State == providers.ServerStateReady {
			ready = append(ready, server.ID)
		}
	}
	if (requireOld && !seen[item.ProviderID]) || len(ready) == 0 {
		return false, nil
	}
	encoded, err := json.Marshal(ready)
	if err != nil {
		return false, err
	}
	var sibling string
	err = tx.QueryRowContext(ctx, `SELECT d.id::text FROM droplets d
 JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id AND r.provider='linode'
 JOIN deployments dep ON dep.account_id=d.account_id AND dep.droplet_id=d.id AND dep.provider_id=d.provider_resource_id
 JOIN panel_instances pi ON pi.account_id=d.account_id AND pi.droplet_id=d.id
 JOIN server_runtime_snapshots rs ON rs.panel_id=pi.id
 JOIN panel_routing_state routing ON routing.panel_id=pi.id
 JOIN user_capacity_snapshots uc ON uc.panel_id=pi.id
 WHERE d.account_id=$1 AND d.id<>$2 AND d.provider_resource_id IN (SELECT jsonb_array_elements_text($3::jsonb))
 AND d.state='READY' AND d.expires_at>now()+interval '10 minutes' AND r.managed AND r.state<>'deleted'
 AND dep.state='PANEL_COMPLETE' AND pi.enabled AND rs.xui_active AND rs.last_error=''
 AND rs.checked_at BETWEEN now()-interval '8 minutes' AND now()
 AND routing.state='APPLIED' AND routing.last_error='' AND routing.verified_at BETWEEN now()-interval '3 minutes' AND now()
 AND uc.target_users>0 AND uc.active_users>=uc.target_users AND uc.deficit=0 AND uc.last_error=''
 AND uc.observed_at BETWEEN now()-interval '3 minutes' AND now()
 AND NOT EXISTS(SELECT 1 FROM lifecycle_deficit_claims previous JOIN droplets deleted ON deleted.id=previous.droplet_id
 LEFT JOIN deployments backfill ON backfill.id=deleted.replacement_deployment_id AND backfill.account_id=previous.account_id
 WHERE previous.account_id=$1 AND previous.recovered_at IS NULL AND deleted.state='DELETED' AND backfill.droplet_id IS DISTINCT FROM d.id)
 ORDER BY d.expires_at DESC,d.id LIMIT 1 FOR SHARE OF d,r,dep,pi,rs,routing,uc`, item.AccountID, item.ID, string(encoded)).Scan(&sibling)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE lifecycle_deficit_claims cl SET recovered_at=now() FROM droplets old,deployments dep
 WHERE cl.droplet_id=old.id AND cl.account_id=$1 AND cl.recovered_at IS NULL AND old.state='DELETED'
 AND dep.id=old.replacement_deployment_id AND dep.account_id=$1 AND dep.droplet_id=$2 AND dep.state='PANEL_COMPLETE'`, item.AccountID, sibling)
	return err == nil, err
}

func recentExpiryProof(at, now time.Time, age time.Duration) bool {
	return !at.IsZero() && !at.After(now) && now.Sub(at) <= age
}

// A claim does not authorize a remote mutation. Recheck all proof immediately
// before the durable start boundary. After started_at commits, ordinary journal
// recovery owns the outcome and provider-confirmed receipts remain replayable.
func (c Container) admitExpiredDeficitRetirement(ctx context.Context, item droplets.LifecycleItem, claimID string) (bool, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+item.AccountID); err != nil {
			return false, err
		}
	}
	var started sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT started_at FROM lifecycle_deficit_claims WHERE droplet_id=$1 AND account_id=$2 AND provider_resource_id=$3 AND claim_id=$4 FOR UPDATE`, item.ID, item.AccountID, item.ProviderID, claimID).Scan(&started)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if started.Valid {
		return true, tx.Commit()
	}
	item.State = droplets.Retiring
	ok, err := expiryDeficitProofTx(ctx, tx, item, true)
	if err != nil {
		return false, err
	}
	if !ok {
		// Fresh observations may recover without changing the claim. Explicit
		// expiry/ownership/account revocations release only an unstarted claim.
		var revoked bool
		err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM droplets d JOIN accounts a ON a.id=d.account_id
  JOIN resources r ON r.account_id=d.account_id AND r.provider_resource_id=d.provider_resource_id
  WHERE d.id=$1 AND d.account_id=$2 AND d.provider_resource_id=$3 AND d.state='RETIRING'
  AND d.expires_at<=now() AND a.enabled AND a.provider='linode' AND r.managed AND r.state<>'deleted')
 AND NOT EXISTS(SELECT 1 FROM operations WHERE account_id=$2 AND idempotency_key='lifecycle-delete:'||$1::text)`, item.ID, item.AccountID, item.ProviderID).Scan(&revoked)
		if err != nil {
			return false, err
		}
		if revoked {
			res, updateErr := tx.ExecContext(ctx, `UPDATE droplets SET state='EXPIRING',updated_at=now() WHERE id=$1 AND account_id=$2 AND provider_resource_id=$3 AND state='RETIRING'`, item.ID, item.AccountID, item.ProviderID)
			if updateErr != nil {
				return false, updateErr
			}
			n, updateErr := res.RowsAffected()
			if updateErr != nil {
				return false, updateErr
			}
			// An identity change must retain the unstarted fence. Never emit a
			// transition or release authorization for a row that was not reverted.
			if n != 1 {
				return false, tx.Commit()
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'EXPIRING')`, item.AccountID, item.ID); err != nil {
				return false, err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM lifecycle_deficit_claims WHERE droplet_id=$1 AND claim_id=$2 AND started_at IS NULL`, item.ID, claimID); err != nil {
				return false, err
			}
		}
		return false, tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `UPDATE lifecycle_deficit_claims SET started_at=now() WHERE droplet_id=$1 AND claim_id=$2 AND started_at IS NULL`, item.ID, claimID)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Called by the existing local consistency pass. It observes readiness only;
// it does not create deployments, retire servers, or call a provider.
func (c Container) reconcileExpiryDeficitBackfill(ctx context.Context) error {
	rows, err := c.DB.QueryContext(ctx, `SELECT cl.droplet_id::text,cl.account_id::text,cl.provider_resource_id FROM lifecycle_deficit_claims cl
 JOIN droplets d ON d.id=cl.droplet_id AND d.account_id=cl.account_id AND d.provider_resource_id=cl.provider_resource_id
 WHERE cl.started_at IS NOT NULL AND cl.recovered_at IS NULL AND d.state='DELETED' AND d.replacement_deployment_id IS NOT NULL`)
	if err != nil {
		return err
	}
	var items []droplets.LifecycleItem
	for rows.Next() {
		var item droplets.LifecycleItem
		if err = rows.Scan(&item.ID, &item.AccountID, &item.ProviderID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err = c.recordExpiryDeficitRecovery(ctx, item); err != nil {
			return err
		}
	}
	return nil
}
func (c Container) recordExpiryDeficitRecovery(ctx context.Context, item droplets.LifecycleItem) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, prefix := range []string{"deployment-admission:", "lifecycle-excess:"} {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+item.AccountID); err != nil {
			return err
		}
	}
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT provider='linode' AND enabled AND provider_state='ACTIVE' AND COALESCE(provider_error_state,'')='' FROM accounts WHERE id=$1 FOR UPDATE`, item.AccountID).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil || !allowed {
		return err
	}
	if err = lockManagedPopulationTx(ctx, tx, item.AccountID); err != nil {
		return err
	}
	if _, err = expiryDeficitServingProofTx(ctx, tx, item, false); err != nil {
		return err
	}
	return tx.Commit()
}
