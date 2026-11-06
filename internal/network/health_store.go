package network

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrHealthStore = errors.New("proxy health store failed")
var ErrHealthStoreBusy = fmt.Errorf("%w: configuration busy or no longer available; update deferred", ErrHealthStore)

type HealthStore interface {
	Save(ctx context.Context, proxyID string, state HealthState) error
}

type SQLHealthStore struct{ DB *sql.DB }

func (s SQLHealthStore) Save(ctx context.Context, proxyID string, state HealthState) error {
	if s.DB == nil || proxyID == "" {
		return ErrHealthStore
	}
	// Browser/authorization sessions hold a configuration share lock. Health
	// persistence must never wait for that session and stall the control worker.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	res, err := s.DB.ExecContext(ctx, `WITH writable AS (SELECT id FROM proxies WHERE id=$1 FOR UPDATE SKIP LOCKED) UPDATE proxies AS p SET status=$2, exit_ip=NULLIF($3,'')::inet, failure_count=$4, consecutive_successes=$5, last_checked_at=$6, last_success_at=NULLIF($7, '0001-01-01 00:00:00+00')::timestamptz, latency_ms=$8, health_error=NULLIF($9,''), updated_at=now() FROM writable AS w WHERE p.id=w.id`, proxyID, state.Status, state.LastExitIP, state.ConsecutiveFailures, state.ConsecutiveSuccesses, state.LastCheckedAt, state.LastSuccessAt, state.LastLatency.Milliseconds(), NormalizeHealthDiagnostic(state.LastError))
	if err != nil {
		return ErrHealthStore
	}
	if n, err := res.RowsAffected(); err != nil {
		return ErrHealthStore
	} else if n != 1 {
		return ErrHealthStoreBusy
	}
	return nil
}
