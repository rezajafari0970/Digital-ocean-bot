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
	rows, err := s.DB.QueryContext(ctx, `SELECT id::text,account_id::text,COALESCE(provider_resource_id,''),COALESCE(profile_id::text,''),COALESCE(replacement_deployment_id::text,''),state,COALESCE(ready_at,created_at),COALESCE(expires_at,updated_at),updated_at FROM droplets WHERE (state IN ('RETIRING','DELETING') OR (state IN ('READY','EXPIRING') AND expires_at IS NOT NULL AND expires_at <= $1::timestamptz + make_interval(secs => GREATEST(300, LEAST(1800, COALESCE((SELECT percentile_cont(0.75) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (done.created_at - d2.created_at))) FROM deployments d2 JOIN LATERAL (SELECT de.created_at FROM deployment_events de WHERE de.deployment_id=d2.id AND de.step='done' AND de.state='READY' ORDER BY de.created_at DESC LIMIT 1) done ON true WHERE d2.account_id=droplets.account_id AND d2.state='READY' AND done.created_at>d2.created_at),600)::int + 120))))) ORDER BY COALESCE(expires_at,updated_at) LIMIT $2`, now, limit)
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
