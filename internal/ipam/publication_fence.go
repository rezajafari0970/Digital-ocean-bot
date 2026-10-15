package ipam

import (
	"context"
	"database/sql"
)

// Order: account-mutation -> account-route -> sorted panel-config -> routing control
// -> sorted routing rows -> endpoints. Matches account purge outer order.
// Row locks also fence writers without advisory locks; persistPlan marks APPLYING
// before remote changes. Never call inside an existing panel-config callback.
func publicationFence(ctx context.Context, tx *sql.Tx, account string) error {
	if _, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))", "account-route:"+account); e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, "SELECT id::text FROM panel_instances WHERE account_id=$1 ORDER BY id", account)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))", "panel-config:"+id); e != nil {
			return e
		}
	}
	var revision int64
	if e = tx.QueryRowContext(ctx, "SELECT revision FROM residential_routing_control WHERE singleton FOR SHARE").Scan(&revision); e != nil {
		return e
	}
	rows, e = tx.QueryContext(ctx, "SELECT rs.panel_id FROM panel_routing_state rs JOIN panel_instances pi ON pi.id=rs.panel_id WHERE pi.account_id=$1 ORDER BY rs.panel_id FOR SHARE OF rs,pi", account)
	if e != nil {
		return e
	}
	for rows.Next() {
	}
	e = rows.Err()
	rows.Close()
	return e
}
