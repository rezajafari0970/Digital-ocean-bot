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
		// Heal missing/stale installer references before evaluating the gate so a
		// healthy account cannot starve indefinitely because of profile drift.
		if err := e.ensureAutomationInstallerRef(ctx, x.ProfileID); err != nil {
			log.Printf("scheduler skip account=%s reason=automation_repair_failed profile=%s err=%v", x.AccountID, x.ProfileID, err)
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		if err := e.ensureAutomationDatabaseTemplate(ctx, x.ProfileID); err != nil {
			log.Printf("scheduler skip account=%s reason=database_template_repair_failed profile=%s err=%v", x.AccountID, x.ProfileID, err)
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		var automationReady bool
		_ = e.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deployment_profiles p JOIN installers i ON i.name=p.config->'installer_ref'->>'name' AND i.version=(p.config->'installer_ref'->>'version')::int WHERE p.id=$1 AND p.enabled=true AND i.active=true AND i.manifest @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb)`, x.ProfileID).Scan(&automationReady)
		if !automationReady {
			log.Printf("scheduler skip account=%s reason=automation_not_ready profile=%s", x.AccountID, x.ProfileID)
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
			log.Printf("scheduler skip account=%s reason=capacity_read err=%v", x.AccountID, capErr)
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Active deployments that already own a droplet count in managed. Only pre-create deployments are pending capacity.
		var preCreate int
		_ = e.DB.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE account_id=$1 AND profile_id=$2 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') AND droplet_id IS NULL`, x.AccountID, x.ProfileID).Scan(&preCreate)
		effectiveOccupancy := managed + preCreate
		if cap.InUse > effectiveOccupancy {
			effectiveOccupancy = cap.InUse
		}
		needed := desired - effectiveOccupancy
		if needed <= 0 {
			log.Printf("scheduler skip account=%s reason=desired_satisfied desired=%d managed=%d provider_inuse=%d effective=%d unmaterialized=%d", x.AccountID, desired, managed, cap.InUse, effectiveOccupancy, preCreate)
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		if x.MaxConcurrent > 0 && concurrent >= x.MaxConcurrent {
			log.Printf("scheduler skip account=%s reason=max_concurrent concurrent=%d max=%d", x.AccountID, concurrent, x.MaxConcurrent)
			_ = e.Leases.Complete(ctx, x, now)
			continue
		}
		// Creation cadence is independent from the scheduler wake-up cadence.
		// At most one new deployment starts per configured spacing window.
		if nextBuild.Valid && now.Before(nextBuild.Time) {
			log.Printf("scheduler skip account=%s reason=build_spacing next=%s", x.AccountID, nextBuild.Time.UTC().Format(time.RFC3339))
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
		if available < 1 && cap.Pending == 0 {
			_, _ = e.DB.ExecContext(ctx, `UPDATE provider_capacity_observations p SET source='vultr_api_saturation',probe_in_flight=false,probe_after=now(),updated_at=now() WHERE p.account_id=$1 AND p.source='vultr_api_probe' AND p.probe_in_flight=true AND p.updated_at < now()-interval '2 minutes' AND NOT EXISTS (SELECT 1 FROM operations o WHERE o.account_id=p.account_id AND o.kind='CREATE_DROPLET' AND o.state IN ('running','unknown','verifying'))`, x.AccountID)
			var probeDue bool
			_ = e.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_capacity_observations pco JOIN accounts a ON a.id=pco.account_id WHERE pco.account_id=$1 AND a.provider='vultr' AND pco.probe_in_flight=false AND ((pco.source='vultr_api_saturation' AND pco.probe_after IS NOT NULL AND pco.probe_after<=now()) OR pco.source='vultr_api_probe_success'))`, x.AccountID).Scan(&probeDue)
			if probeDue {
				available = 1
			}
		}
		if allowed > available {
			allowed = available
		}
		if allowed < 1 {
			log.Printf("scheduler skip account=%s reason=no_available_capacity known=%v limit=%d inuse=%d pending=%d", x.AccountID, cap.LimitKnown, cap.Limit, cap.InUse, cap.Pending)
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

func (e Engine) ensureAutomationInstallerRef(ctx context.Context, profileID string) error {
	var ready bool
	if err := e.DB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM deployment_profiles p
		JOIN installers i ON i.name=p.config->'installer_ref'->>'name'
		 AND i.version=(p.config->'installer_ref'->>'version')::int
		WHERE p.id=$1 AND p.enabled=true AND i.active=true
		 AND i.manifest @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb
	)`, profileID).Scan(&ready); err != nil {
		return err
	}
	if ready {
		return nil
	}
	var name string
	var version int
	if err := e.DB.QueryRowContext(ctx, `SELECT name,version FROM installers
		WHERE active=true AND manifest @> '{"capabilities":["xui_database","xui_panel"]}'::jsonb
		ORDER BY version DESC,name ASC LIMIT 1`).Scan(&name, &version); err != nil {
		return err
	}
	_, err := e.DB.ExecContext(ctx, `UPDATE deployment_profiles
		SET config=jsonb_set(config,'{installer_ref}',jsonb_build_object('name',$2::text,'version',$3::int),true),updated_at=now()
		WHERE id=$1 AND enabled=true`, profileID, name, version)
	return err
}

func (e Engine) ensureAutomationDatabaseTemplate(ctx context.Context, profileID string) error {
	var ready bool
	if err := e.DB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM deployment_profiles p
		JOIN xui_database_templates t ON t.id::text=p.config->>'database_template_id'
		WHERE p.id=$1 AND p.enabled=true AND t.active=true
	)`, profileID).Scan(&ready); err != nil {
		return err
	}
	if ready {
		return nil
	}
	var id string
	if err := e.DB.QueryRowContext(ctx, `SELECT id::text FROM xui_database_templates
		WHERE active=true ORDER BY version DESC,name ASC LIMIT 1`).Scan(&id); err != nil {
		return err
	}
	_, err := e.DB.ExecContext(ctx, `UPDATE deployment_profiles
		SET config=jsonb_set(config,'{database_template_id}',to_jsonb($2::text),true),updated_at=now()
		WHERE id=$1 AND enabled=true`, profileID, id)
	return err
}
