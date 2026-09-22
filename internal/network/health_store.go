package network

import (
	"context"
	"database/sql"
	"errors"
)

var ErrHealthStore = errors.New("proxy health store failed")

type HealthStore interface {
	Save(ctx context.Context, proxyID string, state HealthState) error
}

type SQLHealthStore struct{ DB *sql.DB }

func (s SQLHealthStore) Save(ctx context.Context, proxyID string, state HealthState) error {
	if s.DB == nil || proxyID == "" {
		return ErrHealthStore
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE proxies SET status=$2, exit_ip=NULLIF($3,'')::inet, failure_count=$4, last_checked_at=$5, last_success_at=NULLIF($6, '0001-01-01 00:00:00+00')::timestamptz, updated_at=now() WHERE id=$1`, proxyID, state.Status, state.LastExitIP, state.ConsecutiveFailures, state.LastCheckedAt, state.LastSuccessAt)
	if err != nil {
		return ErrHealthStore
	}
	return nil
}
