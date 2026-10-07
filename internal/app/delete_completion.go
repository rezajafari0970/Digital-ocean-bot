package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

// completeRecoveredDelete is called only after a successful provider absence
// read through the existing account network and generation guards.
func (c Container) completeRecoveredDelete(ctx context.Context, operationID, accountID, providerID string, version int64) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE operations
		SET state='succeeded',error_code=NULL,error_message=NULL,lock_version=lock_version+1,updated_at=now()
		WHERE id=$1 AND account_id=$2 AND resource_id=$3 AND kind='DELETE_DROPLET'
		AND lock_version=$4 AND state IN ('running','unknown','verifying')`,
		operationID, accountID, providerID, version)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return errors.New("operation changed during delete completion")
	}
	// Lifecycle receipts must identify this exact local target. Generic owned
	// resource cleanup may legitimately have no droplet row; keep that path.
	var key string
	if err = tx.QueryRowContext(ctx, "SELECT idempotency_key FROM operations WHERE id=$1", operationID).Scan(&key); err != nil {
		return err
	}
	var dropletID string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM droplets
		WHERE account_id=$1 AND provider_resource_id=$2`, accountID, providerID).Scan(&dropletID)
	if err == nil {
		if key != "lifecycle-delete:"+dropletID {
			return errors.New("delete receipt does not match lifecycle target")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	} else if strings.HasPrefix(key, "lifecycle-delete:") {
		return errors.New("delete receipt has no lifecycle target")
	}
	if err = confirmDeletedTx(ctx, tx, accountID, providerID); err != nil {
		return err
	}
	return tx.Commit()
}

// reconcileCompletedDelete repairs historical gaps using an exact durable
// succeeded receipt. It performs no network request and never invents success
// from a timeout, an inventory count, or an unknown operation outcome.
func (c Container) reconcileCompletedDelete(ctx context.Context, item droplets.LifecycleItem) (bool, error) {
	if item.ProviderID == "" {
		return false, nil
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT o.id::text FROM operations o
		JOIN droplets d ON d.id=$3 AND d.account_id=o.account_id AND d.provider_resource_id=o.resource_id
		WHERE o.account_id=$1 AND o.resource_id=$2 AND o.kind='DELETE_DROPLET'
		AND o.idempotency_key='lifecycle-delete:' || d.id::text AND o.state='succeeded'
		AND d.state IN ('RETIRING','DELETING','DELETED') FOR UPDATE OF o`,
		item.AccountID, item.ProviderID, item.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = confirmDeletedTx(ctx, tx, item.AccountID, item.ProviderID); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
