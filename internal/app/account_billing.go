package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// RefreshOneBillingAccount stays out of request handlers and shares the guarded
// provider transport. Failed observations retain the last data with a stale marker.
func (c Container) RefreshOneBillingAccount(ctx context.Context) error {
	if _, err := c.DB.ExecContext(ctx, `INSERT INTO account_billing_snapshots(account_id) SELECT id FROM accounts WHERE deleted_at IS NULL AND deletion_requested_at IS NULL ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	var id string
	err := c.DB.QueryRowContext(ctx, `UPDATE account_billing_snapshots SET attempted_at=now(),next_attempt_at=now()+interval '5 minutes'
 WHERE account_id=(SELECT b.account_id FROM account_billing_snapshots b JOIN accounts a ON a.id=b.account_id WHERE b.next_attempt_at<=now() AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL ORDER BY b.next_attempt_at FOR UPDATE OF b SKIP LOCKED LIMIT 1) RETURNING account_id::text`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	billing, err := c.readAccountBilling(ctx, id)
	if err != nil {
		code := "billing unavailable; check account network and billing read permission"
		_, saveErr := c.DB.ExecContext(ctx, "UPDATE account_billing_snapshots SET last_error=$2 WHERE account_id=$1", id, code)
		return saveErr
	}
	raw, err := json.Marshal(billing)
	if err != nil {
		return err
	}
	_, err = c.DB.ExecContext(ctx, "UPDATE account_billing_snapshots SET data=$2,observed_at=now(),last_error='' WHERE account_id=$1", id, raw)
	return err
}
func (c Container) readAccountBilling(ctx context.Context, id string) (providers.Billing, error) {
	if err := c.ensureActiveAccountProxy(ctx, id); err != nil {
		return providers.Billing{}, err
	}
	if err := c.requireAccountNetworkReady(ctx, id); err != nil {
		return providers.Billing{}, err
	}
	cfg, err := c.Accounts.AccountForCleanup(ctx, id)
	if err != nil {
		return providers.Billing{}, err
	}
	rt, err := c.runtimeFromConfig(ctx, cfg)
	if err != nil {
		return providers.Billing{}, err
	}
	reader, ok := rt.Driver.(providers.BillingReader)
	if !ok {
		return providers.Billing{}, errors.New("billing unsupported")
	}
	return reader.Billing(ctx)
}
