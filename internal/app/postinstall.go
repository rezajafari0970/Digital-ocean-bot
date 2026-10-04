package app

import (
	"context"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

var ErrPostInstallHostMissing = errors.New("post-install host missing")

func (c Container) PostInstallWorkflow(ctx context.Context, cfg DeploymentConfig) (workflow.Engine, error) {
	sshClient := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: c.DB}}
	database := sanaei.DatabaseManager{Secrets: c.Secrets, Runner: sshClient, Uploader: sshClient}
	panel := sanaei.PanelConfigurer{DB: c.DB, Secrets: c.Secrets, Runner: sshClient, Uploader: sshClient}
	steps := workflow.RuntimeSteps{
		DB:       c.DB,
		Database: database,
		Panel: workflow.PanelConfigureFunc(func(ctx context.Context, d workflow.Deployment) error {
			if d.Host == "" {
				return ErrPostInstallHostMissing
			}
			t := cfg.Target
			t.AccountID = d.AccountID
			t.DropletID = d.DropletID
			t.Host = d.Host
			return panel.Configure(ctx, d.AccountID, d.DropletID, t)
		}),
		Target: cfg.Target, Template: cfg.Template, DatabasePaths: cfg.DatabasePaths,
	}
	return workflow.Engine{Store: workflow.SQLStore{DB: c.DB}, Steps: steps, Finalizer: deploymentReadyFinalizer{DB: c.DB, Lifetime: cfg.Profile.Lifetime}, FailureFinalizer: deploymentFailureFinalizer{DB: c.DB}, RunLease: workflow.PostgresRunLease{DB: c.DB}, PostInstallOnly: true}, nil
}
