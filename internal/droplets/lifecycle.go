package droplets

import (
	"context"
	"database/sql"
	"time"
)

type LifecycleItem struct {
	ID                      string
	AccountID               string
	ProviderID              string
	ProfileID               string
	ReplacementDeploymentID string
	State                   State
	ReadyAt                 time.Time
	ExpiresAt               time.Time
	UpdatedAt               time.Time
}
type LifecycleStore struct{ DB *sql.DB }

func (s LifecycleStore) Due(ctx context.Context, now time.Time, limit int) ([]LifecycleItem, error) {
	if limit < 1 {
		limit = 100
	}
	// Never retire a healthy server before its declared expiry. Replacement
	// preparation must not shorten the server lifetime; Output independently
	// hides configs 10 seconds before expires_at.
	rows, err := s.DB.QueryContext(ctx, `WITH due AS (
		SELECT d.id,d.account_id,d.provider_resource_id,d.profile_id,d.replacement_deployment_id,d.state,d.ready_at,d.created_at,d.expires_at,d.updated_at,
			ROW_NUMBER() OVER (PARTITION BY d.account_id ORDER BY CASE d.state WHEN 'DELETING' THEN 0 WHEN 'RETIRING' THEN 1 WHEN 'EXPIRING' THEN 2 ELSE 3 END,COALESCE(d.expires_at,d.updated_at),d.created_at,d.id) AS rn
		FROM droplets d JOIN accounts a ON a.id=d.account_id
		WHERE (d.state IN ('RETIRING','DELETING') OR (d.state IN ('READY','EXPIRING') AND d.expires_at IS NOT NULL AND d.expires_at <= $1::timestamptz))
		  AND (NOT (d.state IN ('RETIRING','DELETING') AND a.provider_state IN ('LOCKED','BILLING_BLOCKED'))
		    OR EXISTS(SELECT 1 FROM operations o WHERE o.account_id=d.account_id
		      AND o.resource_id=d.provider_resource_id AND o.kind='DELETE_DROPLET'
		      AND o.idempotency_key='lifecycle-delete:'||d.id::text AND o.state='succeeded'))
	) SELECT id::text,account_id::text,COALESCE(provider_resource_id,''),COALESCE(profile_id::text,''),COALESCE(replacement_deployment_id::text,''),state,COALESCE(ready_at,created_at),COALESCE(expires_at,updated_at),updated_at FROM due WHERE rn=1
 AND NOT EXISTS(SELECT 1 FROM worker_item_failures f WHERE f.kind='lifecycle' AND f.item_id=due.id::text AND f.next_retry_at>$1::timestamptz)
 AND NOT EXISTS(SELECT 1 FROM worker_recovery_checkpoints c WHERE c.kind='lifecycle' AND c.item_id=due.id::text)
 ORDER BY COALESCE(expires_at,updated_at) LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LifecycleItem
	for rows.Next() {
		var x LifecycleItem
		if err := rows.Scan(&x.ID, &x.AccountID, &x.ProviderID, &x.ProfileID, &x.ReplacementDeploymentID, &x.State, &x.ReadyAt, &x.ExpiresAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s LifecycleStore) Transition(ctx context.Context, id string, from, to State) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE droplets SET state=$3,updated_at=now() WHERE id=$1 AND state=$2`, id, from, to)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (s LifecycleStore) Event(ctx context.Context, item LifecycleItem, state State) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,$3)`, item.AccountID, item.ID, state)
	return err
}
