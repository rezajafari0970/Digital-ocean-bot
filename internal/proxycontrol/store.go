package proxycontrol

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

type SQLStore struct{ DB *sql.DB }

func (s SQLStore) CurrentGeneration(ctx context.Context, accountID, proxyID, provider string) (int64, bool, error) {
	if s.DB == nil {
		return 0, false, errors.New("proxy control store unavailable")
	}
	var generation int64
	err := s.DB.QueryRowContext(ctx, `
SELECT generation
FROM proxy_runtime_state
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
`, accountID, proxyID, provider).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return generation, true, nil
}

func (s SQLStore) Load(ctx context.Context, accountID, proxyID, provider string) (State, error) {
	x := DefaultState(accountID, proxyID, provider)
	if s.DB == nil {
		return x, errors.New("proxy control store unavailable")
	}
	var retry, checked, success, probeLease sql.NullTime
	var errClass, errDetail sql.NullString
	err := s.DB.QueryRowContext(ctx, `
SELECT health_state,circuit_state,consecutive_failures,consecutive_successes,
       retry_after,generation,last_error_class,last_error_detail,last_checked_at,last_success_at,
       half_open_probe_in_flight,half_open_probe_lease_until
FROM proxy_runtime_state
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
`, accountID, proxyID, provider).Scan(
		&x.HealthState, &x.CircuitState, &x.ConsecutiveFailures, &x.ConsecutiveSuccesses,
		&retry, &x.Generation, &errClass, &errDetail, &checked, &success,
		&x.HalfOpenProbeInFlight, &probeLease,
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
	if probeLease.Valid {
		x.HalfOpenProbeLeaseUntil = &probeLease.Time
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
    last_error_class,last_error_detail,last_checked_at,last_success_at,
    half_open_probe_in_flight,half_open_probe_lease_until,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''),$12,$13,$14,$15,now())
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
    half_open_probe_in_flight=EXCLUDED.half_open_probe_in_flight,
    half_open_probe_lease_until=EXCLUDED.half_open_probe_lease_until,
    updated_at=now()
`,
		x.AccountID, x.ProxyID, x.Provider, x.HealthState, x.CircuitState,
		x.ConsecutiveFailures, x.ConsecutiveSuccesses, x.RetryAfter, x.Generation,
		x.LastErrorClass, x.LastErrorDetail, x.LastCheckedAt, x.LastSuccessAt,
		x.HalfOpenProbeInFlight, x.HalfOpenProbeLeaseUntil,
	)
	return err
}

// Acquire atomically enforces CLOSED/OPEN/HALF_OPEN admission.
// CLOSED allows normal parallel traffic. OPEN denies until retry_after.
// Once retry_after expires, exactly one caller receives a half-open probe lease.
func (s SQLStore) Acquire(ctx context.Context, accountID, proxyID, provider string, now time.Time, lease time.Duration) (State, bool, error) {
	x := DefaultState(accountID, proxyID, provider)
	if s.DB == nil {
		return x, false, errors.New("proxy control store unavailable")
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return x, false, err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `
INSERT INTO proxy_runtime_state(account_id,proxy_id,provider)
VALUES($1,$2,$3)
ON CONFLICT(account_id,proxy_id,provider) DO NOTHING
`, accountID, proxyID, provider); err != nil {
		return x, false, err
	}

	var retry, probeLease sql.NullTime
	if err = tx.QueryRowContext(ctx, `
SELECT circuit_state,retry_after,half_open_probe_in_flight,half_open_probe_lease_until,generation
FROM proxy_runtime_state
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
FOR UPDATE
`, accountID, proxyID, provider).Scan(
		&x.CircuitState, &retry, &x.HalfOpenProbeInFlight, &probeLease, &x.Generation,
	); err != nil {
		return x, false, err
	}
	if retry.Valid {
		x.RetryAfter = &retry.Time
	}
	if probeLease.Valid {
		x.HalfOpenProbeLeaseUntil = &probeLease.Time
	}

	allowed := false
	switch x.CircuitState {
	case "", "closed":
		allowed = true
	case "open":
		if retry.Valid && now.Before(retry.Time) {
			allowed = false
			break
		}
		if x.HalfOpenProbeInFlight && probeLease.Valid && now.Before(probeLease.Time) {
			allowed = false
			break
		}
		until := now.Add(lease)
		if _, err = tx.ExecContext(ctx, `
UPDATE proxy_runtime_state
SET circuit_state='half_open',half_open_probe_in_flight=true,
    half_open_probe_lease_until=$4,updated_at=now()
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
`, accountID, proxyID, provider, until); err != nil {
			return x, false, err
		}
		x.CircuitState = "half_open"
		x.HalfOpenProbeInFlight = true
		x.HalfOpenProbeLeaseUntil = &until
		allowed = true
	case "half_open":
		if x.HalfOpenProbeInFlight && probeLease.Valid && now.Before(probeLease.Time) {
			allowed = false
			break
		}
		until := now.Add(lease)
		if _, err = tx.ExecContext(ctx, `
UPDATE proxy_runtime_state
SET half_open_probe_in_flight=true,half_open_probe_lease_until=$4,updated_at=now()
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
`, accountID, proxyID, provider, until); err != nil {
			return x, false, err
		}
		x.HalfOpenProbeInFlight = true
		x.HalfOpenProbeLeaseUntil = &until
		allowed = true
	default:
		allowed = false
	}

	if err = tx.Commit(); err != nil {
		return x, false, err
	}
	return x, allowed, nil
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

func (s SQLStore) ApplyObservation(ctx context.Context, accountID, proxyID, provider string, result network.HealthResult, policy Policy) (State, error) {
	x, _, err := s.applyObservation(ctx, accountID, proxyID, provider, 0, false, result, policy)
	return x, err
}

// ApplyObservationForGeneration applies passive transport evidence only while
// the runtime's captured generation still matches the stored generation.
func (s SQLStore) ApplyObservationForGeneration(ctx context.Context, accountID, proxyID, provider string, generation int64, result network.HealthResult, policy Policy) (State, bool, error) {
	return s.applyObservation(ctx, accountID, proxyID, provider, generation, true, result, policy)
}

func (s SQLStore) applyObservation(ctx context.Context, accountID, proxyID, provider string, generation int64, requireGeneration bool, result network.HealthResult, policy Policy) (State, bool, error) {
	x := DefaultState(accountID, proxyID, provider)
	if s.DB == nil {
		return x, false, errors.New("proxy control store unavailable")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return x, false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
INSERT INTO proxy_runtime_state(account_id,proxy_id,provider)
VALUES($1,$2,$3)
ON CONFLICT(account_id,proxy_id,provider) DO NOTHING
`, accountID, proxyID, provider); err != nil {
		return x, false, err
	}
	var retry, checked, success, probeLease sql.NullTime
	var errClass, errDetail sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT health_state,circuit_state,consecutive_failures,consecutive_successes,
       retry_after,generation,last_error_class,last_error_detail,last_checked_at,last_success_at,
       half_open_probe_in_flight,half_open_probe_lease_until
FROM proxy_runtime_state
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
FOR UPDATE
`, accountID, proxyID, provider).Scan(
		&x.HealthState, &x.CircuitState, &x.ConsecutiveFailures, &x.ConsecutiveSuccesses,
		&retry, &x.Generation, &errClass, &errDetail, &checked, &success,
		&x.HalfOpenProbeInFlight, &probeLease,
	)
	if err != nil {
		return x, false, err
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
	if probeLease.Valid {
		x.HalfOpenProbeLeaseUntil = &probeLease.Time
	}
	if errClass.Valid {
		x.LastErrorClass = errClass.String
	}
	if errDetail.Valid {
		x.LastErrorDetail = errDetail.String
	}
	if requireGeneration && x.Generation != generation {
		return x, false, nil
	}
	x = ApplyHealth(x, result, policy)
	_, err = tx.ExecContext(ctx, `
UPDATE proxy_runtime_state SET
 health_state=$4,circuit_state=$5,consecutive_failures=$6,consecutive_successes=$7,
 retry_after=$8,generation=$9,last_error_class=NULLIF($10,''),last_error_detail=NULLIF($11,''),
 last_checked_at=$12,last_success_at=$13,half_open_probe_in_flight=$14,
 half_open_probe_lease_until=$15,updated_at=now()
WHERE account_id=$1 AND proxy_id=$2 AND provider=$3
`, accountID, proxyID, provider, x.HealthState, x.CircuitState, x.ConsecutiveFailures,
		x.ConsecutiveSuccesses, x.RetryAfter, x.Generation, x.LastErrorClass, x.LastErrorDetail,
		x.LastCheckedAt, x.LastSuccessAt, x.HalfOpenProbeInFlight, x.HalfOpenProbeLeaseUntil)
	if err != nil {
		return x, false, err
	}
	if err = tx.Commit(); err != nil {
		return x, false, err
	}
	return x, true, nil
}
