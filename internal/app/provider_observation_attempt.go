package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

// ProviderAttempt binds evidence to its start and the actual secret version,
// including replacement of a credential under the same secret reference.
type ProviderAttempt struct {
	AccountID, SecretRef string
	CredentialAt         sql.NullTime
	Started              time.Time
}

func (c Container) BeginProviderAttempt(ctx context.Context, id string, scheduled bool) (ProviderAttempt, error) {
	a := ProviderAttempt{AccountID: id}
	err := c.DB.QueryRowContext(ctx, `SELECT a.secret_ref,s.updated_at,clock_timestamp() FROM accounts a LEFT JOIN secrets s ON s.account_id=a.id AND s.id=a.secret_ref WHERE a.id=$1`, id).Scan(&a.SecretRef, &a.CredentialAt, &a.Started)
	if err != nil {
		return a, err
	}
	if scheduled {
		_, err = c.DB.ExecContext(ctx, `UPDATE accounts SET provider_refresh_attempt_at=$2 WHERE id=$1`, id, a.Started)
	}
	return a, err
}

func lockProviderAttempt(ctx context.Context, tx *sql.Tx, a ProviderAttempt) (bool, error) {
	var ref string
	var live bool
	err := tx.QueryRowContext(ctx, `SELECT secret_ref,enabled AND deleted_at IS NULL AND deletion_requested_at IS NULL FROM accounts WHERE id=$1 FOR UPDATE`, a.AccountID).Scan(&ref, &live)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !live || ref != a.SecretRef || a.Started.IsZero() {
		return false, nil
	}
	var at time.Time
	err = tx.QueryRowContext(ctx, `SELECT updated_at FROM secrets WHERE account_id=$1 AND id=$2 FOR SHARE NOWAIT`, a.AccountID, ref).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return !a.CredentialAt.Valid, nil
	}
	if err != nil {
		return false, err
	}
	return a.CredentialAt.Valid && at.Equal(a.CredentialAt.Time), nil
}

// Error and semantic evidence have independent ordering. A failed observation
// never resolves a restriction or clears an unrelated network observation.
func (c Container) RecordProviderObservation(ctx context.Context, a ProviderAttempt, state string, cause error, detail string) (bool, error) {
	if state == ProviderStateActive || cause == nil {
		return false, errors.New("error observation requires failure evidence")
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	valid, err := lockProviderAttempt(ctx, tx, a)
	if err != nil || !valid {
		return false, err
	}
	var pe *providers.Error
	contacted := errors.As(cause, &pe) && pe.StatusCode > 0
	var result sql.Result
	if IsProviderObservationError(state) {
		result, err = tx.ExecContext(ctx, `UPDATE accounts SET provider_error_state=$2,provider_error_detail=$3,provider_error_started_at=$4,provider_error_at=clock_timestamp(),
   provider_checked_at=CASE WHEN $5 THEN clock_timestamp() ELSE provider_checked_at END,
   runtime_status=CASE WHEN provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN 'PROVIDER_'||provider_state ELSE $6 END,
   runtime_status_detail=$3,runtime_status_at=clock_timestamp(),next_build_at=NULL,updated_at=clock_timestamp()
   WHERE id=$1 AND COALESCE(provider_error_started_at,'epoch')<=$4 AND COALESCE(provider_success_started_at,'epoch')<=$4`, a.AccountID, state, detail, a.Started, contacted, ProviderStateRuntimeStatus(state))
	} else {
		result, err = recordProviderRestrictionTx(ctx, tx, a, state, contacted, detail)
	}
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	return true, tx.Commit()
}

func recordProviderRestrictionTx(ctx context.Context, tx *sql.Tx, a ProviderAttempt, state string, contacted bool, detail string) (sql.Result, error) {
	return tx.ExecContext(ctx, `UPDATE accounts SET provider_state=$2,provider_state_detail=$3,provider_state_at=clock_timestamp(),provider_restriction_started_at=$4,provider_observation_started_at=GREATEST(provider_observation_started_at,$4),
   provider_checked_at=CASE WHEN $5 THEN clock_timestamp() ELSE provider_checked_at END,
   provider_account_status=CASE WHEN $2='LOCKED' THEN 'locked' ELSE provider_account_status END,
   provider_account_observed_at=CASE WHEN $2='LOCKED' THEN clock_timestamp() ELSE provider_account_observed_at END,
   runtime_status=$6,runtime_status_detail=$3,runtime_status_at=clock_timestamp(),next_build_at=NULL,updated_at=clock_timestamp()
   WHERE id=$1 AND COALESCE(provider_restriction_started_at,CASE WHEN provider_observation_started_at IS NULL AND provider_state IN ('LOCKED','TOKEN_INVALID','PERMISSION_DENIED','BILLING_BLOCKED') THEN provider_state_at END,'epoch')<=$4`, a.AccountID, state, detail, a.Started, contacted, ProviderStateRuntimeStatus(state))
}
