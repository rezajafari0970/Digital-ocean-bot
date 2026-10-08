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
	res, err := s.DB.ExecContext(ctx, `UPDATE proxies SET status=$2, exit_ip=NULLIF($3,'')::inet, failure_count=$4, consecutive_successes=$5, last_checked_at=$6, last_success_at=NULLIF($7, '0001-01-01 00:00:00+00')::timestamptz, latency_ms=$8, health_error=NULLIF($9,''), updated_at=now() WHERE id=$1`, proxyID, state.Status, state.LastExitIP, state.ConsecutiveFailures, state.ConsecutiveSuccesses, state.LastCheckedAt, state.LastSuccessAt, state.LastLatency.Milliseconds(), NormalizeHealthDiagnostic(state.LastError))
	if err != nil {
		return ErrHealthStore
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return ErrHealthStore
	}
	return nil
}
