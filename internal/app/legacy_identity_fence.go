package app

import (
	"context"
	"database/sql"
	"errors"
)

type legacyIdentityFenceKey struct{}
type legacyIdentityFence struct{ Snapshot countrySnapshot }

func (c Container) legacyIdentityContext(ctx context.Context, account string) (context.Context, error) {
	if _, ok := ctx.Value(legacyIdentityFenceKey{}).(*legacyIdentityFence); ok {
		return ctx, nil
	}
	snapshot, err := loadCountrySnapshot(ctx, c.DB, account)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, legacyIdentityFenceKey{}, &legacyIdentityFence{Snapshot: snapshot}), nil
}

// Legacy AUTO keeps its established selection behavior, but each observation
// can publish only against the exact route/configuration from which it began.
// Locks are short: network I/O never occurs in this transaction.
func (c Container) beginLegacyIdentityWrite(ctx context.Context, account string) (*sql.Tx, error) {
	tx, err := countryRouteTx(ctx, c.DB, account)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*sql.Tx, error) { tx.Rollback(); return nil, err }
	fence, _ := ctx.Value(legacyIdentityFenceKey{}).(*legacyIdentityFence)
	if fence == nil {
		return tx, nil
	}
	expected := fence.Snapshot
	// Policy writers and explicit AUTO compatibility leases serialize here.
	var currentFence int64
	err = tx.QueryRowContext(ctx, `SELECT fence FROM account_proxy_selection WHERE account_id=$1 FOR UPDATE NOWAIT`, account).Scan(&currentFence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fail(err)
	}
	current, err := loadCountrySnapshot(ctx, tx, account)
	if err != nil {
		return fail(err)
	}
	if current.Scope != expected.Scope || current.Active != expected.Active || current.Epoch != expected.Epoch || current.Fence != expected.Fence {
		return fail(ErrStaleAccountNetwork)
	}
	if current.Active != "" {
		var id string
		if err = tx.QueryRowContext(ctx, `SELECT id::text FROM proxies WHERE id=$1 FOR SHARE NOWAIT`, current.Active).Scan(&id); err != nil {
			return fail(err)
		}
		if p, ok := current.proxy(current.Active); ok && p.SecretRef != "" {
			var version string
			if err = tx.QueryRowContext(ctx, `SELECT xmin::text||':'||updated_at::text FROM secrets WHERE proxy_id=$1 AND id=$2 FOR SHARE NOWAIT`, p.ID, p.SecretRef).Scan(&version); err != nil {
				return fail(err)
			}
			if version != p.SecretVersion {
				return fail(ErrStaleAccountNetwork)
			}
		}
	}
	return tx, nil
}

func (c Container) commitLegacyIdentityWrite(ctx context.Context, tx *sql.Tx, account string) error {
	fence, _ := ctx.Value(legacyIdentityFenceKey{}).(*legacyIdentityFence)
	var next countrySnapshot
	if fence != nil {
		var err error
		next, err = loadCountrySnapshot(ctx, tx, account)
		if err != nil {
			return err
		}
		// Only an identity/epoch changed by this transaction may advance the fence.
		if next.Scope != fence.Snapshot.Scope || next.Active != fence.Snapshot.Active || next.Fence != fence.Snapshot.Fence {
			return ErrStaleAccountNetwork
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if fence != nil {
		fence.Snapshot = next
	}
	return nil
}

func (c Container) execLegacyIdentity(ctx context.Context, query string, args ...any) (sql.Result, error) {
	fence, _ := ctx.Value(legacyIdentityFenceKey{}).(*legacyIdentityFence)
	if fence == nil {
		return c.DB.ExecContext(ctx, query, args...)
	}
	account := fence.Snapshot.Config.Account
	tx, err := c.beginLegacyIdentityWrite(ctx, account)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if err = c.commitLegacyIdentityWrite(ctx, tx, account); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Container) scanLegacyIdentity(ctx context.Context, account, query string, args []any, dest ...any) error {
	tx, err := c.beginLegacyIdentityWrite(ctx, account)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, query, args...).Scan(dest...); err != nil {
		return err
	}
	return c.commitLegacyIdentityWrite(ctx, tx, account)
}
