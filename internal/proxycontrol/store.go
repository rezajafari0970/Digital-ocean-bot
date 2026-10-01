package proxycontrol

import (
	"context"
	"database/sql"
	"errors"
)

type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Load(ctx context.Context, accountID, proxyID, provider string) (State, error) {
	x := DefaultState(accountID, proxyID, provider)
	if s.DB == nil {
		return x, errors.New("proxy control store unavailable")
	}
	var retry, checked, success sql.NullTime
	var errClass, errDetail sql.NullString
	err := s.DB.QueryRowContext(ctx, `
SELECT health_state,circuit_state,consecutive_failures,consecutive_successes,
       retry_after,generation,last_error_class,last_error_detail,last_checked_at,last_success_at
FROM proxy_runtime_state
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
`, accountID, proxyID, provider).Scan(
		&x.HealthState, &x.CircuitState, &x.ConsecutiveFailures, &x.ConsecutiveSuccesses,
		&retry, &x.Generation, &errClass, &errDetail, &checked, &success,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return x, nil
	}
	if err != nil {
		return x, err
	}
	if retry.Valid {
		x.RetryAfter = &retry.Time
	}
	if checked.Valid {
		x.LastCheckedAt = &checked.Time
	}
	if success.Valid {
		x.LastSuccessAt = &success.Time
	}
	if errClass.Valid {
		x.LastErrorClass = errClass.String
	}
	if errDetail.Valid {
		x.LastErrorDetail = errDetail.String
	}
	return x, nil
}

func (s SQLStore) Save(ctx context.Context, x State) error {
	if s.DB == nil {
		return errors.New("proxy control store unavailable")
	}
	if x.Generation < 1 {
		x.Generation = 1
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO proxy_runtime_state(
    account_id,proxy_id,provider,health_state,circuit_state,
    consecutive_failures,consecutive_successes,retry_after,generation,
    last_error_class,last_error_detail,last_checked_at,last_success_at,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''),$12,$13,now())
ON CONFLICT(account_id,proxy_id,provider) DO UPDATE SET
    health_state=EXCLUDED.health_state,
    circuit_state=EXCLUDED.circuit_state,
    consecutive_failures=EXCLUDED.consecutive_failures,
    consecutive_successes=EXCLUDED.consecutive_successes,
    retry_after=EXCLUDED.retry_after,
    generation=EXCLUDED.generation,
    last_error_class=EXCLUDED.last_error_class,
    last_error_detail=EXCLUDED.last_error_detail,
    last_checked_at=EXCLUDED.last_checked_at,
    last_success_at=EXCLUDED.last_success_at,
    updated_at=now()
`,
		x.AccountID, x.ProxyID, x.Provider, x.HealthState, x.CircuitState,
		x.ConsecutiveFailures, x.ConsecutiveSuccesses, x.RetryAfter, x.Generation,
		x.LastErrorClass, x.LastErrorDetail, x.LastCheckedAt, x.LastSuccessAt,
	)
	return err
}

func (s SQLStore) BumpGeneration(ctx context.Context, accountID, proxyID, provider string) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("proxy control store unavailable")
	}
	var generation int64
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO proxy_runtime_state(account_id,proxy_id,provider,generation)
VALUES($1,$2,$3,1)
ON CONFLICT(account_id,proxy_id,provider) DO UPDATE
SET generation=proxy_runtime_state.generation+1,updated_at=now()
RETURNING generation
`, accountID, proxyID, provider).Scan(&generation)
	return generation, err
}
