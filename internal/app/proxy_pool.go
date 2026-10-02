package app

import (
	"context"
	"database/sql"
	"errors"
)

func (c Container) ensureActiveAccountProxy(ctx context.Context, accountID string) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "account-proxy:"+accountID); err != nil {
		return err
	}

	var mode, current string
	if err = tx.QueryRowContext(ctx, `SELECT np.mode,COALESCE(np.proxy_id::text,'') FROM network_profiles np WHERE np.account_id=$1 FOR UPDATE`, accountID).Scan(&mode, &current); err != nil {
		return err
	}
	if mode != "proxy_required" {
		return tx.Commit()
	}

	var usable bool
	if current != "" {
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_proxy_pool ap JOIN proxies p ON p.id=ap.proxy_id WHERE ap.account_id=$1 AND ap.proxy_id=$2::uuid AND ap.enabled=true AND p.status='healthy')`, accountID, current).Scan(&usable); err != nil {
			return err
		}
	}
	if usable {
		return tx.Commit()
	}

	var next string
	err = tx.QueryRowContext(ctx, `SELECT p.id::text FROM account_proxy_pool ap JOIN proxies p ON p.id=ap.proxy_id WHERE ap.account_id=$1 AND ap.enabled=true AND p.status='healthy' ORDER BY ap.priority,ap.proxy_id LIMIT 1 FOR UPDATE OF ap,p`, accountID).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNetworkNotReady
	}
	if err != nil {
		return err
	}
	if current != next {
		if _, err = tx.ExecContext(ctx, `UPDATE network_profiles SET proxy_id=$2::uuid,updated_at=now() WHERE account_id=$1`, accountID, next); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,timezone,locale,last_health_ok,updated_at) VALUES($1,'UTC','en-US',false,now()) ON CONFLICT(account_id) DO UPDATE SET sticky_session=NULL,fallback_active=false,rotation_started_at=NULL,exit_ip=NULL,subnet_key=NULL,asn=NULL,country=NULL,country_code=NULL,preferred_country=NULL,preferred_country_code=NULL,last_health_ok=false,last_health_at=NULL,updated_at=now()`, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
