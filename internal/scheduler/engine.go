package scheduler

import (
	"context"
	"database/sql"
	"time"
)

type Starter interface {
	StartScheduledDeployment(context.Context, string, string) error
}
type Engine struct {
	DB      *sql.DB
	Store   SQLStore
	Leases  LeaseStore
	Starter Starter
}

func (e Engine) RunDue(ctx context.Context, now time.Time) error {
	items, err := e.Leases.ClaimDue(ctx, now, 100, time.Minute)
	if err != nil {
		return err
	}
	for _, x := range items {
		var limit, active, creating, provisioning int
		_ = e.DB.QueryRowContext(ctx, `SELECT COALESCE((data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, x.AccountID).Scan(&limit)
		_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE state='active'),count(*) FILTER(WHERE state IN ('new','creating')),count(*) FILTER(WHERE state='provisioning') FROM resources WHERE account_id=$1 AND type='droplet'`, x.AccountID).Scan(&active, &creating, &provisioning)
		var reservedReplacement int
		_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FROM droplets WHERE account_id=$1 AND state='EXPIRING' AND replacement_deployment_id IS NULL`, x.AccountID).Scan(&reservedReplacement)
		var concurrent int
		_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE account_id=$1 AND profile_id=$2 AND state NOT IN ('READY','FAILED')`, x.AccountID, x.ProfileID).Scan(&concurrent)
		allowed := x.BatchSize
		if x.MaxConcurrent > 0 && allowed > x.MaxConcurrent-concurrent {
			allowed = x.MaxConcurrent - concurrent
		}
		cap := Capacity{Limit: limit, Active: active, Creating: creating, Provisioning: provisioning, ReservedReplacement: reservedReplacement}
		if allowed > cap.Available() {
			allowed = cap.Available()
		}
		for i := 0; i < allowed; i++ {
			_ = e.Starter.StartScheduledDeployment(ctx, x.AccountID, x.ProfileID)
		}
		_ = e.Leases.Complete(ctx, x, now)
	}
	return nil
}
