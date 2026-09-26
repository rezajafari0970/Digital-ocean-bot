package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"math/rand"
	"time"
)

var ErrProfileDisabled = errors.New("deployment profile disabled")

func (c Container) StartDeployment(ctx context.Context, accountID, profileID string) (workflow.Deployment, error) {
	if err := c.MaintainStickyIdentity(ctx, accountID); err != nil {
		return workflow.Deployment{}, err
	}
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
	var imagesRaw []byte
	var lifetimeMin, lifetimeMax int
	var fallbackAnyRegion bool
	if err := c.DB.QueryRowContext(ctx, `SELECT preferred_regions,preferred_sizes,preferred_images,COALESCE(server_lifetime_min_seconds,server_lifetime_seconds),COALESCE(server_lifetime_max_seconds,server_lifetime_seconds),fallback_any_region FROM accounts WHERE id=$1`, accountID).Scan(&regionsRaw, &sizesRaw, &imagesRaw, &lifetimeMin, &lifetimeMax, &fallbackAnyRegion); err != nil {
		return d, err
	}
	var regions, sizes, images []string
	_ = json.Unmarshal(regionsRaw, &regions)
	_ = json.Unmarshal(sizesRaw, &sizes)
	_ = json.Unmarshal(imagesRaw, &images)
	rand.Shuffle(len(regions), func(i, j int) { regions[i], regions[j] = regions[j], regions[i] })
	if len(regions) > 0 {
		effective.Region = regions[0]
		effective.Regions = regions
	}
	if len(sizes) > 0 {
		effective.Size = sizes[rand.Intn(len(sizes))]
	}
	if fallbackAnyRegion {
		var catalogRaw []byte
		if err := c.DB.QueryRowContext(ctx, `SELECT data FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, accountID).Scan(&catalogRaw); err == nil {
			var catalog struct {
				Regions []struct {
					Slug      string `json:"slug"`
					Available bool   `json:"available"`
				} `json:"Regions"`
				Sizes []struct {
					Slug      string   `json:"slug"`
					Available bool     `json:"available"`
					Regions   []string `json:"regions"`
				} `json:"Sizes"`
			}
			if json.Unmarshal(catalogRaw, &catalog) == nil {
				allowed := map[string]bool{}
				for _, sz := range catalog.Sizes {
					if sz.Slug == effective.Size && sz.Available {
						for _, rg := range sz.Regions {
							allowed[rg] = true
						}
						break
					}
				}
				// Keep only selected regions where the randomly selected plan is offered.
				compatible := effective.Regions[:0]
				for _, rg := range effective.Regions {
					if allowed[rg] {
						compatible = append(compatible, rg)
					}
				}
				effective.Regions = compatible
				if len(effective.Regions) > 0 {
					effective.Region = effective.Regions[0]
				}
				seen := map[string]bool{}
				for _, rg := range effective.Regions {
					seen[rg] = true
				}
				for _, rg := range catalog.Regions {
					if rg.Available && allowed[rg.Slug] && !seen[rg.Slug] {
						effective.Regions = append(effective.Regions, rg.Slug)
						seen[rg.Slug] = true
					}
				}
			}
		}
	}
	if len(images) > 0 {
		effective.Image = images[rand.Intn(len(images))]
	}
	if lifetimeMin > 0 {
		if lifetimeMax < lifetimeMin {
			lifetimeMax = lifetimeMin
		}
		seconds := lifetimeMin
		if lifetimeMax > lifetimeMin {
			seconds += rand.Intn(lifetimeMax - lifetimeMin + 1)
		}
		effective.Lifetime = time.Duration(seconds) * time.Second
	}
	if len(effective.InstallSteps) == 0 {
		if len(effective.InstallScriptRefs) > 0 {
			resolved, resolveErr := (provisioning.ScriptRegistry{DB: c.DB}).Resolve(ctx, effective.InstallScriptRefs)
			if resolveErr != nil {
				return d, resolveErr
			}
			effective.InstallSteps = resolved
		} else {
			effective.InstallSteps = append([]provisioning.ScriptStep(nil), defaultProvisionPlan().Scripts...)
		}
	}
	if err := profiles.AttachSnapshot(ctx, d.ID, effective); err != nil {
		return d, err
	}
	// Every deployment receives its own SSH key pair before the provider
	// mutation. Recovery reuses the persisted identity rather than rotating it.
	snap, err := profiles.SnapshotForDeployment(ctx, d.ID)
	if err != nil {
		return d, err
	}
	if err := c.ensureDeploymentSSHIdentity(ctx, accountID, d, &snap); err != nil {
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
	return engine.Run(ctx, workflow.Request{DeploymentID: d.ID, AccountID: accountID, ProfileID: profileID, ClientCount: profile.Config.ClientCount, InboundID: profile.Config.InboundID, EmailPrefix: profile.Config.EmailPrefix})
}
