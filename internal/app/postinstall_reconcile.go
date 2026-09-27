package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

var ErrPostInstallNotProvablyComplete = errors.New("post-install completion not provable")

func (c Container) ReconcileCompletedPostInstall(ctx context.Context, deploymentID, accountID string) error {
	if err := c.RequirePostInstallCapabilities(ctx, deploymentID); err != nil {
		return err
	}
	d, err := (workflow.SQLStore{DB: c.DB}).Get(ctx, deploymentID, accountID)
	if err != nil {
		return err
	}
	var ok bool
	err = c.DB.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM xui_database_deployments x WHERE x.droplet_id=$1 AND x.generation=$2 AND x.state='COMPLETED')
 AND
 EXISTS(SELECT 1 FROM xui_panel_deployments p WHERE p.droplet_id=$1 AND p.generation=$2 AND p.state='COMPLETED')`, d.DropletID, d.PostInstallGeneration).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPostInstallNotProvablyComplete
	}
	cfg, _, err := c.DeploymentConfigFromSnapshot(ctx, deploymentID)
	if err != nil {
		return err
	}
	if err = (deploymentReadyFinalizer{DB: c.DB, Lifetime: cfg.Profile.Lifetime}).MarkReady(ctx, d); err != nil {
		return err
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE deployments SET state='PANEL_COMPLETE',current_step='panel_complete',last_error='',updated_at=now() WHERE id=$1 AND account_id=$2 AND postinstall_generation=$3`, deploymentID, accountID, d.PostInstallGeneration)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrPostInstallNotProvablyComplete
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,'panel','PANEL_COMPLETE','RECONCILED_FROM_COMPLETED_LEDGERS')`, deploymentID); err != nil {
		return err
	}
	return tx.Commit()
}
