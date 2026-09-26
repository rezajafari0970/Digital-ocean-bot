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
	rows, err := s.DB.QueryContext(ctx, `SELECT id::text,account_id::text,kind,state,updated_at FROM operations WHERE (state IN ('running','verifying') AND updated_at <= now()-interval '15 seconds') OR (state='unknown' AND updated_at <= now()-interval '30 seconds') ORDER BY updated_at LIMIT $1`, limit)
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
	rows, err := s.DB.QueryContext(ctx, `SELECT id::text,account_id::text,'deployment',state,updated_at FROM deployments WHERE state NOT IN ('READY','FAILED','WAITING_INSTALLER','INSTALL_COMPLETE') OR (state='WAITING_INSTALLER' AND (profile_snapshot->'installer_ref' IS NOT NULL OR EXISTS(SELECT 1 FROM deployment_installer_selections s WHERE s.deployment_id=deployments.id))) ORDER BY updated_at LIMIT $1`, limit)
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
