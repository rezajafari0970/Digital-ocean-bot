package resilience

import (
	"context"
	"database/sql"
	"time"
)

type RuntimeState struct {
	AccountID     string
	CircuitState  CircuitState
	Failures      int
	RetryAfter    *time.Time
	RateRemaining *int
	RateResetAt   *time.Time
}
type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Load(ctx context.Context, id string) (RuntimeState, error) {
	var x RuntimeState
	x.AccountID = id
	var retry, reset sql.NullTime
	var remaining sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT circuit_state,consecutive_failures,retry_after,rate_remaining,rate_reset_at FROM account_runtime_state WHERE account_id=$1`, id).Scan(&x.CircuitState, &x.Failures, &retry, &remaining, &reset)
	if err == sql.ErrNoRows {
		return x, nil
	}
	if retry.Valid {
		x.RetryAfter = &retry.Time
	}
	if reset.Valid {
		x.RateResetAt = &reset.Time
	}
	if remaining.Valid {
		v := int(remaining.Int64)
		x.RateRemaining = &v
	}
	return x, err
}
func (s SQLStore) Save(ctx context.Context, x RuntimeState) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO account_runtime_state(account_id,circuit_state,consecutive_failures,retry_after,rate_remaining,rate_reset_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,now()) ON CONFLICT(account_id) DO UPDATE SET circuit_state=EXCLUDED.circuit_state,consecutive_failures=EXCLUDED.consecutive_failures,retry_after=EXCLUDED.retry_after,rate_remaining=EXCLUDED.rate_remaining,rate_reset_at=EXCLUDED.rate_reset_at,updated_at=now()`, x.AccountID, x.CircuitState, x.Failures, x.RetryAfter, x.RateRemaining, x.RateResetAt)
	return err
}
