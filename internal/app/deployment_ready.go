package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

type deploymentReadyFinalizer struct {
	DB       *sql.DB
	Lifetime time.Duration
}

func (f deploymentReadyFinalizer) MarkReady(ctx context.Context, d workflow.Deployment) error {
	if f.DB == nil || d.DropletID == "" {
		return workflow.ErrRuntimeConfig
	}
	seconds := int64(f.Lifetime / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	_, err := f.DB.ExecContext(ctx, `UPDATE droplets SET state='READY',ready_at=COALESCE(ready_at,now()),expires_at=COALESCE(expires_at,now()+($3 * interval '1 second')),updated_at=now() WHERE id=$1 AND account_id=$2 AND state IN ('PROVISIONING','READY')`, d.DropletID, d.AccountID, seconds)
	return err
}

type deploymentFailureFinalizer struct{ DB *sql.DB }

func (f deploymentFailureFinalizer) MarkFailed(ctx context.Context, d workflow.Deployment) error {
	if f.DB == nil || d.DropletID == "" {
		return nil
	}
	res, err := f.DB.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE id=$1 AND account_id=$2 AND state='PROVISIONING'`, d.DropletID, d.AccountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		_, err = f.DB.ExecContext(ctx, `INSERT INTO lifecycle_events(id,account_id,resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')`, d.AccountID, d.DropletID)
	}
	return err
}
