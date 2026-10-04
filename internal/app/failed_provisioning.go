package app

import "context"

// ReconcileFailedProvisioning repairs a crash/missing failure finalizer after
// a terminal deployment was committed. Only bot-owned provisioning rows qualify;
// READY servers and deployments that were rearmed are never retired here.
func (c Container) ReconcileFailedProvisioning(ctx context.Context) (int64, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT d.id::text,dr.id::text,dr.account_id::text
 FROM deployments d JOIN droplets dr ON dr.id=d.droplet_id AND dr.account_id=d.account_id
 WHERE dr.state='PROVISIONING' AND dr.ready_at IS NULL AND d.state IN('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')
 AND d.updated_at<now()-interval '5 minutes'
 AND NOT EXISTS(SELECT 1 FROM deployments other WHERE other.droplet_id=dr.id AND other.id<>d.id AND other.state NOT IN('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK'))
 ORDER BY d.updated_at LIMIT 8 FOR UPDATE OF d,dr SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type item struct{ dep, drop, account string }
	var items []item
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.dep, &x.drop, &x.account); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	var retired int64
	for _, x := range items {
		result, e := tx.ExecContext(ctx, "UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND state='PROVISIONING'", x.drop)
		if e != nil {
			return 0, e
		}
		affected, e := result.RowsAffected()
		if e != nil {
			return 0, e
		}
		if affected == 0 {
			continue
		}
		retired++

		if _, err = tx.ExecContext(ctx, "INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')", x.account, x.drop); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,'failure-finalizer','FAILED','Terminal deployment reconciled to lifecycle retirement')", x.dep); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return retired, nil
}
