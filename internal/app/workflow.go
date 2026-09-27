package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
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
	executor := droplets.Executor{Operations: runtime.Operations, Provider: runtime.Provider, Gate: runtime.Gate}
	if runtime.Gateway != nil &&
		!proxyAdapterByName(runtime.Config.ProxyAdapter).Capabilities().StickySession {

		eg := &network.EgressGuard{
			Client: runtime.Gateway.Client,
		}

		executor.EgressCheck = func(ctx context.Context) error {
			_, err := eg.Observe(ctx)
			return err
		}
	}
	sshClient := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: c.DB}}
	provisionStore := provisioning.SQLStore{DB: c.DB}
	provisioner := provisioning.Engine{Store: provisionStore, Secrets: c.Secrets, SSH: sshClient, Events: provisionStore, Scripts: provisioning.SSHScriptRunner{SSH: sshClient}, Readiness: provisioning.ReadinessCollector{SSH: sshClient, Recorder: provisionStore}}
	database := sanaei.DatabaseManager{Secrets: c.Secrets, Runner: sshClient, Uploader: sshClient}
	panel := sanaei.PanelConfigurer{DB: c.DB, Secrets: c.Secrets, Runner: sshClient, Uploader: sshClient}
	steps := workflow.RuntimeSteps{DB: c.DB, Droplets: executor, Waiter: workflow.DigitalOceanWaiter{Provider: runtime.Provider}, Provisioner: provisioner, Database: database, Panel: workflow.PanelConfigureFunc(func(ctx context.Context, d workflow.Deployment) error {
		t := cfg.Target
		t.AccountID = d.AccountID
		t.DropletID = d.DropletID
		if t.Host == "" && d.ProviderID != "" {
			info, e := workflow.DigitalOceanWaiter{Provider: runtime.Provider}.Wait(ctx, d.ProviderID)
			if e != nil {
				return e
			}
			t.Host = info.Host
		}
		return panel.Configure(ctx, d.AccountID, d.DropletID, t)
	}), Profile: cfg.Profile, ProvisionPlan: cfg.Provision, Target: cfg.Target, Template: cfg.Template, DatabasePaths: cfg.DatabasePaths}
	return workflow.Engine{Store: workflow.SQLStore{DB: c.DB}, Steps: steps, Finalizer: deploymentReadyFinalizer{DB: c.DB, Lifetime: cfg.Profile.Lifetime}, FailureFinalizer: deploymentFailureFinalizer{DB: c.DB}, RunLease: workflow.PostgresRunLease{DB: c.DB}}, nil
}
