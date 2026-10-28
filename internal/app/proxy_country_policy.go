package app

import (
	"context"
	"database/sql"
	"fmt"
)

// FenceCountryWriterTx invalidates older candidate publishers and authorizes
// only this transaction to change an explicit account's committed identity.
func FenceCountryWriterTx(ctx context.Context, tx *sql.Tx, account string) (int64, error) {
	var fence int64
	err := tx.QueryRowContext(ctx, `INSERT INTO account_proxy_selection(account_id,fence) VALUES($1,1)
 ON CONFLICT(account_id) DO UPDATE SET fence=account_proxy_selection.fence+1,legacy_lease_until=NULL,updated_at=now() RETURNING fence`, account).Scan(&fence)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `SELECT set_config('dob.country_writer',$1,true)`, fmt.Sprintf("%s:%d", account, fence))
	return fence, err
}

func countryPolicyReady(ctx context.Context, db countryQuery, account string) (bool, error) {
	var ready bool
	err := db.QueryRowContext(ctx, `SELECT NOT EXISTS(
 SELECT 1 FROM account_proxy_country_policies p
 LEFT JOIN account_proxy_selection s ON s.account_id=p.account_id
 LEFT JOIN account_network_identities i ON i.account_id=p.account_id
 WHERE p.account_id=$1 AND p.countries<>'["AUTO"]'::jsonb
 AND (s.applied_revision IS DISTINCT FROM p.revision OR s.active_stage='' OR
      NOT (p.countries ? s.active_stage) OR
 (s.active_stage='AUTO' AND i.selected_target_country IS NOT NULL) OR
 (s.active_stage<>'AUTO' AND (i.selected_target_country IS DISTINCT FROM s.active_stage OR upper(i.country_code) IS DISTINCT FROM s.active_stage))))`, account).Scan(&ready)
	return ready, err
}

func explicitCountryPolicy(ctx context.Context, db countryQuery, account string) (bool, error) {
	var explicit bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_proxy_country_policies WHERE account_id=$1 AND countries<>'["AUTO"]'::jsonb)`, account).Scan(&explicit)
	return explicit, err
}

type countryAutoRecoveryKey struct{}
