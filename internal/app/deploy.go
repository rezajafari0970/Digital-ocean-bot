package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"time"
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
	effective := profile.Config
	var regionsRaw, sizesRaw []byte
	var image string
	var lifetimeSeconds int
	if err := c.DB.QueryRowContext(ctx, `SELECT preferred_regions,preferred_sizes,COALESCE(preferred_image,''),server_lifetime_seconds FROM accounts WHERE id=$1`, accountID).Scan(&regionsRaw, &sizesRaw, &image, &lifetimeSeconds); err != nil {
		return d, err
	}
	var regions, sizes []string
	_ = json.Unmarshal(regionsRaw, &regions)
	_ = json.Unmarshal(sizesRaw, &sizes)
	if len(regions) > 0 {
		effective.Region = regions[0]
		effective.Regions = regions
	}
	if len(sizes) > 0 {
		effective.Size = sizes[0]
	}
	if image != "" {
		effective.Image = image
	}
	if lifetimeSeconds > 0 {
		effective.Lifetime = time.Duration(lifetimeSeconds) * time.Second
	}
	if err := profiles.AttachSnapshot(ctx, d.ID, effective); err != nil {
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
