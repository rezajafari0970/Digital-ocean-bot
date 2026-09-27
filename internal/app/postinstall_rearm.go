package app

import (
	"context"
	"database/sql"
	"errors"
)

var ErrPostInstallRearmNotAllowed = errors.New("post-install rearm not allowed")

func (c Container) RearmPostInstall(ctx context.Context, deploymentID, accountID string) error {
	if err := c.RequirePostInstallCapabilities(ctx, deploymentID); err != nil {
		return err
	}
	tx, err := c.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, step, dropletID string
	var generation int
	err = tx.QueryRowContext(ctx, `SELECT state,current_step,COALESCE(droplet_id::text,''),postinstall_generation FROM deployments WHERE id=$1 AND account_id=$2 FOR UPDATE`, deploymentID, accountID).Scan(&state, &step, &dropletID, &generation)
	if err != nil {
		return err
	}
	if state != "FAILED" || step != "done" || dropletID == "" {
		return ErrPostInstallRearmNotAllowed
	}
	var failed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xui_database_deployments WHERE droplet_id=$1 AND generation=$2 AND state='FAILED')`, dropletID, generation).Scan(&failed)
	if err != nil {
		return err
	}
	if !failed {
		return ErrPostInstallRearmNotAllowed
	}
	res, err := tx.ExecContext(ctx, `UPDATE deployments SET postinstall_generation=postinstall_generation+1,state='IMPORTING_DATABASE',current_step='database',last_error=NULL,updated_at=now() WHERE id=$1 AND account_id=$2 AND postinstall_generation=$3`, deploymentID, accountID, generation)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrPostInstallRearmNotAllowed
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,'database','IMPORTING_DATABASE',$2)`, deploymentID, "POSTINSTALL_REARM")
	if err != nil {
		return err
	}
	return tx.Commit()
}
