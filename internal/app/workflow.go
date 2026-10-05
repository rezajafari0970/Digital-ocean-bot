package app

import (
	"context"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

type DeploymentConfig struct {
	Profile       droplets.Profile
	Provision     provisioning.Plan
	Target        provisioning.Target
	Template      sanaei.DatabaseTemplate
	DatabasePaths sanaei.DatabasePaths
}

func (c Container) Workflow(ctx context.Context, accountID string, cfg DeploymentConfig) (workflow.Engine, error) {
	runtime, err := c.Runtime(ctx, accountID)
	if err != nil {
		return workflow.Engine{}, err
	}
	compute, err := computeDriver(runtime)
	if err != nil {
		return workflow.Engine{}, err
	}
	executor := droplets.Executor{Operations: runtime.Operations, Provider: compute, Gate: runtime.Gate}
	executor.PreCreateCheck = func(checkCtx context.Context) error {
		var desired, managed, preCreate int
		err := c.DB.QueryRowContext(checkCtx, `SELECT (SELECT desired_server_count FROM accounts WHERE id=$1),(SELECT count(*) FROM droplets WHERE account_id=$1 AND state<>'DELETED'),(SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') AND droplet_id IS NULL)`, accountID).Scan(&desired, &managed, &preCreate)
		cap, capErr := capacity.Read(checkCtx, c.DB, accountID, 2*time.Minute)
		effectiveOccupancy := managed + preCreate
		if capErr == nil {
			effectiveOccupancy = desiredEffectiveOccupancy(managed, preCreate, cap.InUse)
		}
		if err != nil || capErr != nil || !desiredOccupancyAllowsReserved(desired, effectiveOccupancy) {
			if runtime.Config.Provider == "vultr" {
				_, _ = c.DB.ExecContext(checkCtx, `UPDATE provider_capacity_observations SET source='vultr_api_saturation',probe_in_flight=false,probe_after=now(),updated_at=now() WHERE account_id=$1 AND source='vultr_api_probe' AND probe_in_flight=true`, accountID)
			}
			return droplets.ErrMutationBlocked
		}
		return nil
	}
	executor.OnCreateError = func(cbCtx context.Context, createErr error) {
		if blockErr := capacity.RecordCreateBlock(cbCtx, c.DB, accountID, createErr); blockErr != nil {
			log.Printf("create restriction persistence account=%s: %v", accountID, blockErr)
		}
		class := providers.Class(createErr)
		if class == providers.ErrorAuthentication || class == providers.ErrorPermissionDenied || class == providers.ErrorAccountLocked || class == providers.ErrorRateLimited || class == providers.ErrorTransport || class == providers.ErrorUnavailable || class == providers.ErrorAmbiguousOutcome {
			state := ClassifyAccountProviderError(createErr, runtime.Config.Network.Mode == network.RouteProxyRequired)
			c.RecordProviderObservation(cbCtx, accountID, state, createErr, createErr.Error())
		}
		if runtime.Config.Provider == "vultr" {
			c.handleVultrCreateError(cbCtx, accountID, compute, createErr)
		}
	}
	if runtime.Config.Provider == "vultr" {
		executor.OnCreateSuccess = func(cbCtx context.Context, _ providers.CreateServerResult) {
			c.recordVultrCreateSuccess(cbCtx, accountID, compute)
		}
	}
	var eg *network.EgressGuard
	if runtime.Gateway != nil && !proxyAdapterByName(runtime.Config.ProxyAdapter).Capabilities().StickySession {
		eg = &network.EgressGuard{Client: runtime.Gateway.Client}
	}
	executor.EgressCheck = func(ctx context.Context) error {
		if err := c.EnsureFreshNetworkIdentity(ctx, accountID); err != nil {
			return err
		}
		if eg != nil {
			if _, err := eg.Observe(ctx); err != nil {
				return err
			}
		}
		return runtime.CheckMutationGeneration(ctx)
	}
	sshClient := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: c.DB}}
	provisionStore := provisioning.SQLStore{DB: c.DB}
	provisioner := provisioning.Engine{Store: provisionStore, Secrets: c.Secrets, SSH: sshClient, Events: provisionStore, Scripts: provisioning.SSHScriptRunner{SSH: sshClient}, Readiness: provisioning.ReadinessCollector{SSH: sshClient, Recorder: provisionStore}}
	database := sanaei.DatabaseManager{Secrets: c.Secrets, Runner: sshClient, Uploader: sshClient}
	panel := sanaei.PanelConfigurer{DB: c.DB, Secrets: c.Secrets, Runner: sshClient, Uploader: sshClient}
	steps := workflow.RuntimeSteps{DB: c.DB, Droplets: executor, Waiter: workflow.ServerWaiter{Provider: compute}, Provisioner: provisioner, Database: database, Panel: workflow.PanelConfigureFunc(func(ctx context.Context, d workflow.Deployment) error {
		t := cfg.Target
		t.AccountID = d.AccountID
		t.DropletID = d.DropletID
		if t.Host == "" && d.ProviderID != "" {
			info, e := workflow.ServerWaiter{Provider: compute}.Wait(ctx, d.ProviderID)
			if e != nil {
				return e
			}
			t.Host = info.Host
		}
		return panel.Configure(ctx, d.AccountID, d.DropletID, t)
	}), Profile: cfg.Profile, ProvisionPlan: cfg.Provision, Target: cfg.Target, Template: cfg.Template, DatabasePaths: cfg.DatabasePaths}
	return workflow.Engine{Store: workflow.SQLStore{DB: c.DB}, Steps: steps, Finalizer: deploymentReadyFinalizer{DB: c.DB, Lifetime: cfg.Profile.Lifetime}, FailureFinalizer: deploymentFailureFinalizer{DB: c.DB}, RunLease: workflow.PostgresRunLease{DB: c.DB}}, nil
}
