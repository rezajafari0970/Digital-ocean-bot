package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

var ErrProfileDisabled = errors.New("deployment profile disabled")

func (c Container) StartDeployment(ctx context.Context, accountID, profileID string) (workflow.Deployment, error) {
	profiles := workflow.ProfileStore{DB: c.DB}
	profile, err := profiles.Get(ctx, profileID)
	if err != nil {
		return workflow.Deployment{}, err
	}
	if profile.AccountID != accountID || !profile.Enabled {
		return workflow.Deployment{}, ErrProfileDisabled
	}
	store := workflow.SQLStore{DB: c.DB}
	d, _, err := store.Reserve(ctx, workflow.Request{AccountID: accountID, ProfileID: profileID, ClientCount: profile.Config.ClientCount, InboundID: profile.Config.InboundID, EmailPrefix: profile.Config.EmailPrefix})
	if err != nil {
		return d, err
	}
	if err := profiles.AttachSnapshot(ctx, d.ID, profile.Config); err != nil {
		return d, err
	}
	cfg, _, err := c.DeploymentConfigFromSnapshot(ctx, d.ID)
	if err != nil {
		return d, err
	}
	engine, err := c.Workflow(ctx, accountID, cfg)
	if err != nil {
		return d, err
	}
	return engine.Run(ctx, workflow.Request{AccountID: accountID, ProfileID: profileID, ClientCount: profile.Config.ClientCount, InboundID: profile.Config.InboundID, EmailPrefix: profile.Config.EmailPrefix})
}
