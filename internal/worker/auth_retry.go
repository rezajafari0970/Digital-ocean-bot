package worker

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

const ProviderAuthRetryMarker = "PROVIDER_AUTH_RETRY"

// Only a fresh, typed provider rejection can slow retained reconciliation.
// The existing checkpoint supplies the attempt start, so a response from an
// older credential cannot postpone work after a validated replacement.
func providerAuthRetryTx(ctx context.Context, tx *sql.Tx, kind, item, account string, cause error) (time.Duration, error) {
	if account == "" || (kind != "operation" && kind != "lifecycle") {
		return 0, nil
	}
	var p *providers.Error
	if !errors.As(cause, &p) || p.Class != providers.ErrorAuthentication || p.StatusCode != 401 {
		return 0, nil
	}
	var enabled bool
	if err := tx.QueryRowContext(ctx, `SELECT enabled AND (canary_account_id IS NULL OR canary_account_id::text=$1) FROM proxy_economy_policy WHERE singleton`, account).Scan(&enabled); err != nil {
		return 0, err
	}
	if !enabled {
		return 0, nil
	}
	var changed, started sql.NullTime
	// Credential replacement updates/locks this account row before rearming
	// failures. Take the same row before the failure ledger to serialize both
	// completion orders without touching checkpoint or operation ownership.
	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id=$1 FOR SHARE`, account).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	// A separate READ COMMITTED statement gets the post-lock secret snapshot
	// when a credential-save transaction committed while the row lock waited.
	err = tx.QueryRowContext(ctx, `SELECT s.updated_at,c.created_at FROM accounts a
LEFT JOIN secrets s ON s.account_id=a.id AND s.id=a.secret_ref
LEFT JOIN worker_recovery_checkpoints c ON c.account_id=a.id AND c.kind=$2 AND c.item_id=$3
WHERE a.id=$1`, account, kind, item).Scan(&changed, &started)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !started.Valid || !changed.Valid {
		return 0, nil
	}
	if !changed.Time.Before(started.Time) {
		return 2 * time.Second, nil
	}
	return 5 * time.Minute, nil
}

// Called only from the validated credential-save transaction. Keep counters,
// retained work and native checkpoints; move exact authentication retries due.
func RearmProviderAuthRetriesTx(ctx context.Context, tx *sql.Tx, account string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&exists); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE worker_item_failures SET next_retry_at=now()
WHERE account_id=$1 AND kind IN ('operation','lifecycle') AND last_error=$2`, account, ProviderAuthRetryMarker)
	return err
}
