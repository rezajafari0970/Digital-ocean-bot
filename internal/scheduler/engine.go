package scheduler

import (
	"context"
	"database/sql"
	"math/rand"
	"time"
)

type Starter interface {
	PrepareScheduledAccount(context.Context, string) error
	StartScheduledDeployment(context.Context, string, string) error
}

func providerAllowsCreate(enabled bool, runtimeStatus string) bool {
	return enabled && runtimeStatus == "READY"
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
		// Refresh/validate sticky egress before READY is evaluated. This lets
		// ISOLATION_WAIT accounts recover automatically on the next scheduler tick.
		_ = e.Starter.PrepareScheduledAccount(ctx, x.AccountID)
		var enabled bool
		var runtimeStatus string
		if err := e.DB.QueryRowContext(ctx, `SELECT enabled,runtime_status FROM accounts WHERE id=$1`, x.AccountID).Scan(&enabled, &runtimeStatus); err != nil || !enabled || runtimeStatus != "READY" {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		var limit, providerDroplets, pendingCreates, concurrent, desired, managed, spacingMin, spacingMax int
		var snapshotAt time.Time
		var nextBuild sql.NullTime
		err := e.DB.QueryRowContext(ctx, `SELECT
			COALESCE((SELECT (ps.data->'Limits'->>'DropletLimit')::int FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0),
			COALESCE((SELECT jsonb_array_length(COALESCE(ps.data->'Droplets','[]'::jsonb)) FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0),
			(SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state IN ('planned','running','verifying','unknown') AND COALESCE(resource_id,'')=''),
			(SELECT count(*) FROM deployments WHERE account_id=$1 AND profile_id=$2 AND state NOT IN ('READY','FAILED')),
			(SELECT desired_server_count FROM accounts WHERE id=$1),
			(SELECT count(*) FROM droplets WHERE account_id=$1 AND state NOT IN ('DELETED')),
			(SELECT build_spacing_minutes FROM accounts WHERE id=$1),
			(SELECT build_spacing_max_minutes FROM accounts WHERE id=$1),
			(SELECT next_build_at FROM accounts WHERE id=$1),
			COALESCE((SELECT ps.created_at FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),'epoch'::timestamptz)`, x.AccountID, x.ProfileID).Scan(&limit, &providerDroplets, &pendingCreates, &concurrent, &desired, &managed, &spacingMin, &spacingMax, &nextBuild, &snapshotAt)
		if err != nil {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Never make a CREATE capacity decision from stale provider state.
		// Refresh is deliberately not triggered here: scheduler must fail closed,
		// leaving provider I/O to the explicit refresh/sync paths.
		if snapshotAt.Equal(time.Unix(0, 0)) || now.Sub(snapshotAt) > 2*time.Minute {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Active deployments that already own a droplet count in managed. Only pre-create deployments are pending capacity.
		var preCreate int
		_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE account_id=$1 AND profile_id=$2 AND state NOT IN ('READY','FAILED') AND COALESCE(provider_id,'')=''`, x.AccountID, x.ProfileID).Scan(&preCreate)
		needed := desired - managed - preCreate
		if needed <= 0 {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Creation cadence is independent from the scheduler wake-up cadence.
		// At most one new deployment starts per configured spacing window.
		if nextBuild.Valid && now.Before(nextBuild.Time) {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		allowed := 1
		if allowed > needed {
			allowed = needed
		}
		// Provider snapshot is the source of truth for occupied capacity. Only
		// create operations without a provider resource consume additional slots;
		// this avoids double-counting droplets already visible at DigitalOcean.
		available := limit - providerDroplets - pendingCreates
		if available < 0 {
			available = 0
		}
		if allowed > available {
			allowed = available
		}
		for i := 0; i < allowed; i++ {
			if err := e.Starter.StartScheduledDeployment(ctx, x.AccountID, x.ProfileID); err == nil {
				if spacingMax < spacingMin {
					spacingMax = spacingMin
				}
				minutes := spacingMin
				if spacingMax > spacingMin {
					minutes += rand.Intn(spacingMax - spacingMin + 1)
				}
				_, _ = e.DB.ExecContext(ctx, `UPDATE accounts SET next_build_at=$2 WHERE id=$1`, x.AccountID, now.Add(time.Duration(minutes)*time.Minute))
			}
		}
		_ = e.Leases.Complete(ctx, x, now)
	}
	return nil
}
