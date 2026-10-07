package worker

import (
	"context"
	"database/sql"
	"time"
)

type RecoveryItem struct {
	ID        string
	AccountID string
	Kind      string
	State     string
	UpdatedAt time.Time
}
type RecoveryStore struct{ DB *sql.DB }

func (s RecoveryStore) Operations(ctx context.Context, limit int) ([]RecoveryItem, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT o.id::text,o.account_id::text,o.kind,o.state,o.updated_at
		FROM operations o
		JOIN accounts a ON a.id=o.account_id
		WHERE (((o.state IN ('running','verifying') AND o.updated_at <= now()-interval '15 seconds') OR (o.state='unknown' AND o.updated_at <= now()-interval '30 seconds')))
		  AND NOT (o.kind='DELETE_DROPLET' AND a.provider_state IN ('LOCKED','BILLING_BLOCKED'))
 AND NOT EXISTS(SELECT 1 FROM worker_recovery_checkpoints c WHERE c.kind='operation' AND c.item_id=o.id::text)
 AND NOT EXISTS(SELECT 1 FROM worker_item_failures f WHERE f.kind='operation' AND f.item_id=o.id::text AND f.next_retry_at>now())
		ORDER BY row_number() OVER(PARTITION BY o.account_id ORDER BY o.updated_at,o.id),o.updated_at,o.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoveryItem
	for rows.Next() {
		var x RecoveryItem
		if err := rows.Scan(&x.ID, &x.AccountID, &x.Kind, &x.State, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s RecoveryStore) Deployments(ctx context.Context, limit int) ([]RecoveryItem, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT d.id::text,d.account_id::text,'deployment',d.state,d.updated_at FROM deployments d LEFT JOIN worker_item_failures f ON f.kind='deployment' AND f.item_id=d.id::text WHERE ((d.state='WAITING_INSTALLER' AND (d.profile_snapshot->'installer_ref' IS NOT NULL OR EXISTS(SELECT 1 FROM deployment_installer_selections s WHERE s.deployment_id=d.id AND s.generation=d.installer_generation))) OR (d.state IN ('INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL') AND EXISTS(SELECT 1 FROM installer_runs ir WHERE ir.deployment_id=d.id AND ir.generation=d.installer_generation AND ir.state='INSTALL_COMPLETE' AND ir.manifest_snapshot @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb)) OR d.state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING'))
 AND NOT EXISTS(SELECT 1 FROM worker_recovery_checkpoints c WHERE c.kind='deployment' AND c.item_id=d.id::text)
 AND (f.next_retry_at IS NULL OR f.next_retry_at<=now())
 ORDER BY row_number() OVER(PARTITION BY d.account_id ORDER BY d.updated_at,d.id),d.updated_at,d.id LIMIT $1`, limit)
	return scanRecoveryDeployments(rows, err)
}
func (s RecoveryStore) DeferredDeploymentPage(ctx context.Context, limit int, after string) ([]RecoveryItem, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT d.id::text,d.account_id::text,'deployment',d.state,d.updated_at FROM deployments d LEFT JOIN worker_item_failures f ON f.kind='deployment' AND f.item_id=d.id::text WHERE ((d.state='WAITING_INSTALLER' AND (d.profile_snapshot->'installer_ref' IS NOT NULL OR EXISTS(SELECT 1 FROM deployment_installer_selections s WHERE s.deployment_id=d.id AND s.generation=d.installer_generation))) OR (d.state IN ('INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL') AND EXISTS(SELECT 1 FROM installer_runs ir WHERE ir.deployment_id=d.id AND ir.generation=d.installer_generation AND ir.state='INSTALL_COMPLETE' AND ir.manifest_snapshot @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb)) OR d.state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING'))
 AND NOT EXISTS(SELECT 1 FROM worker_recovery_checkpoints c WHERE c.kind='deployment' AND c.item_id=d.id::text)
 AND f.next_retry_at>now()
 AND f.last_error<>'RECOVERY_INTERRUPTED_BEFORE_DURABLE_COMPLETION' AND d.current_step='provision'
 AND ($2='' OR d.id>NULLIF($2,'')::uuid) ORDER BY d.id LIMIT $1`, limit, after)
	return scanRecoveryDeployments(rows, err)
}
func scanRecoveryDeployments(rows *sql.Rows, err error) ([]RecoveryItem, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoveryItem
	for rows.Next() {
		var x RecoveryItem
		if err := rows.Scan(&x.ID, &x.AccountID, &x.Kind, &x.State, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
