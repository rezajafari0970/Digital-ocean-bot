package scheduler

import (
	"context"
	"database/sql"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"log"
	"math/rand"
	"time"
)

type Starter interface {
	PrepareScheduledAccount(context.Context, string) error
	StartScheduledDeployment(context.Context, string, string) error
}

func providerAllowsCreate(enabled bool, runtimeStatus, providerState, providerError string) bool {
	return enabled && runtimeStatus == "READY" && providerState == "ACTIVE" && providerError == ""
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
		var runtimeStatus, providerState, providerError string
		if err := e.DB.QueryRowContext(ctx, `SELECT enabled,runtime_status,provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, x.AccountID).Scan(&enabled, &runtimeStatus, &providerState, &providerError); err != nil || !providerAllowsCreate(enabled, runtimeStatus, providerState, providerError) {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Scheduled automation is allowed only for a fully pinned, capable installer.
		var automationReady bool
		_ = e.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deployment_profiles p JOIN installers i ON i.name=p.config->'installer_ref'->>'name' AND i.version=(p.config->'installer_ref'->>'version')::int WHERE p.id=$1 AND p.enabled=true AND i.active=true AND i.manifest @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb)`, x.ProfileID).Scan(&automationReady)
		if !automationReady {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		var concurrent, desired, managed, spacingMin, spacingMax int
		var nextBuild sql.NullTime
		err := e.DB.QueryRowContext(ctx, `SELECT
			(SELECT count(*) FROM deployments d WHERE d.account_id=$1 AND d.profile_id=$2 AND d.state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL','REGISTERING_CLIENTS','REGISTERING_TRAFFIC') AND (d.droplet_id IS NULL OR EXISTS(SELECT 1 FROM droplets r WHERE r.id=d.droplet_id AND r.state<>'DELETED'))),
			(SELECT desired_server_count FROM accounts WHERE id=$1),
			(SELECT count(*) FROM droplets WHERE account_id=$1 AND state NOT IN ('DELETED')),
			(SELECT build_spacing_minutes FROM accounts WHERE id=$1),
			(SELECT build_spacing_max_minutes FROM accounts WHERE id=$1),
			(SELECT next_build_at FROM accounts WHERE id=$1)`, x.AccountID, x.ProfileID).Scan(&concurrent, &desired, &managed, &spacingMin, &spacingMax, &nextBuild)
		if err != nil {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		cap, capErr := capacity.Read(ctx, e.DB, x.AccountID, 2*time.Minute)
		if capErr != nil {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Active deployments that already own a droplet count in managed. Only pre-create deployments are pending capacity.
		var preCreate int
		_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE account_id=$1 AND profile_id=$2 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') AND COALESCE(provider_id,'')=''`, x.AccountID, x.ProfileID).Scan(&preCreate)
		needed := desired - managed - preCreate
		if needed <= 0 {
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		if x.MaxConcurrent > 0 && concurrent >= x.MaxConcurrent {
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
		available := cap.Available()
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
			} else {
				log.Printf("scheduler start deployment failed account=%s profile=%s err=%v", x.AccountID, x.ProfileID, err)
			}
		}
		_ = e.Leases.Complete(ctx, x, now)
	}
	return nil
}
