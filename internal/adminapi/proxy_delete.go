package adminapi

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

var errProxyAssignmentsChanged = errors.New("proxy assignments changed; retry deletion")

// removeProxy separates residential use from account API transport. Removing
// residential use preserves a shared account proxy. Deleting the proxy itself
// detaches accounts without changing their fail-closed network mode.
func (s *Server) removeProxy(ctx context.Context, id string, residentialOnly bool) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout='5s'"); err != nil {
		return false, err
	}
	if residentialOnly {
		if _, err = tx.ExecContext(ctx, "DELETE FROM residential_proxies WHERE proxy_id=$1", id); err != nil {
			return false, err
		}
		var retained bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM proxies WHERE id=$1)", id).Scan(&retained); err != nil {
			return false, err
		}
		return retained, tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT account_id::text FROM network_profiles WHERE proxy_id=$1 UNION SELECT account_id::text FROM account_proxy_pool WHERE proxy_id=$1 ORDER BY 1`, id)
	if err != nil {
		return false, err
	}
	ids := []string{}
	for rows.Next() {
		var a string
		if err = rows.Scan(&a); err != nil {
			rows.Close()
			return false, err
		}
		ids = append(ids, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, a := range ids {
		for _, prefix := range []string{"account-proxy:", "account-route:"} {
			if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", prefix+a); err != nil {
				return false, err
			}
		}
	}
	var found string
	err = tx.QueryRowContext(ctx, "SELECT id::text FROM proxies WHERE id=$1 FOR UPDATE", id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	var changed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM network_profiles WHERE proxy_id=$1 AND NOT(account_id::text=ANY($2::text[])) UNION ALL SELECT 1 FROM account_proxy_pool WHERE proxy_id=$1 AND NOT(account_id::text=ANY($2::text[])))`, id, pq.Array(ids)).Scan(&changed)
	if err != nil {
		return false, err
	}
	if changed {
		return false, errProxyAssignmentsChanged
	}
	for _, q := range []string{
		`UPDATE account_transport_state SET transport_epoch=transport_epoch+1,active_proxy_id=NULL,transition_reason='proxy-deleted',updated_at=now() WHERE account_id::text=ANY($1::text[])`,
		`UPDATE account_network_identities SET sticky_session=NULL,exit_ip=NULL,subnet_key=NULL,last_health_ok=false,last_health_at=NULL,rotation_started_at=NULL,updated_at=now() WHERE account_id::text=ANY($1::text[])`,
	} {
		if _, err = tx.ExecContext(ctx, q, pq.Array(ids)); err != nil {
			return false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE network_profiles SET proxy_id=NULL,updated_at=now() WHERE proxy_id=$1`, id); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM proxies WHERE id=$1", id); err != nil {
		return false, err
	}
	return false, tx.Commit()
}
