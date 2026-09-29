package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"math/rand"
	"strings"

	"github.com/lib/pq"
	"time"
)

var ErrProfileDisabled = errors.New("deployment profile disabled")
var ErrDatabaseTemplateNotConfigured = errors.New("database template not configured")
var ErrDatabaseTemplateUnavailable = errors.New("database template unavailable")

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
	if profile.Config.DatabaseTemplateID == "" {
		return workflow.Deployment{}, ErrDatabaseTemplateNotConfigured
	}
	var templateActive bool
	var templatePath string
	if err := c.DB.QueryRowContext(ctx, `SELECT active,storage_path FROM xui_database_templates WHERE id=$1`, profile.Config.DatabaseTemplateID).Scan(&templateActive, &templatePath); err != nil || !templateActive || templatePath == "" || templatePath == "pending" {
		return workflow.Deployment{}, ErrDatabaseTemplateUnavailable
	}
	// Serialize capacity admission per account. The capacity check and PLANNED
	// deployment reservation share one transaction, so the next caller observes
	// this reservation before it can consume the same provider slot.
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Deployment{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "deployment-admission:"+accountID); err != nil {
		return workflow.Deployment{}, err
	}
	var enabled bool
	var runtimeStatus, providerState, providerError string
	if err = tx.QueryRowContext(ctx, `SELECT enabled,runtime_status,provider_state,COALESCE(provider_error_state,'') FROM accounts WHERE id=$1`, accountID).Scan(&enabled, &runtimeStatus, &providerState, &providerError); err != nil || !enabled || runtimeStatus != "READY" || providerState != ProviderStateActive || providerError != "" {
		return workflow.Deployment{}, ErrCapacityUnavailable
	}
	cap, capErr := capacity.Read(ctx, tx, accountID, 2*time.Minute)
	if capErr != nil {
		return workflow.Deployment{}, ErrCapacitySnapshotStale
	}
	if (cap.LimitKnown && cap.Limit < 1) || cap.Available() < 1 {
		return workflow.Deployment{}, ErrCapacityUnavailable
	}
	store := workflow.SQLStore{DB: tx}
	d, _, err := store.Reserve(ctx, workflow.Request{AccountID: accountID, ProfileID: profileID, ClientCount: profile.Config.ClientCount, InboundID: profile.Config.InboundID, EmailPrefix: profile.Config.EmailPrefix})
	if err != nil {
		return d, err
	}
	if err = tx.Commit(); err != nil {
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
	var catalog providers.Catalog
	var catalogRaw []byte
	if err := c.DB.QueryRowContext(ctx, `SELECT canonical FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1`, accountID).Scan(&catalogRaw); err == nil {
		var obs providers.Observation
		if json.Unmarshal(catalogRaw, &obs) == nil {
			catalog = obs.Catalog
		}
	}
	if fallbackAnyRegion {
		allowed := map[string]bool{}
		for _, plan := range catalog.Plans {
			if plan.ID == effective.Size && plan.Available {
				for _, rg := range plan.AvailableRegions {
					allowed[rg] = true
				}
				break
			}
		}
		compatible := make([]string, 0, len(effective.Regions))
		seen := map[string]bool{}
		for _, rg := range effective.Regions {
			if allowed[rg] {
				compatible = append(compatible, rg)
				seen[rg] = true
			}
		}
		for _, rg := range catalog.Regions {
			if rg.Available && allowed[rg.ID] && !seen[rg.ID] {
				compatible = append(compatible, rg.ID)
				seen[rg.ID] = true
			}
		}
		effective.Regions = compatible
		if len(compatible) > 0 {
			effective.Region = compatible[0]
		}
	}
	if len(images) > 0 {
		available := map[string]bool{}
		for _, im := range catalog.Images {
			if im.Available && im.Family == "ubuntu" && (im.Version == "22.04" || im.Version == "24.04" || im.Version == "26.04") {
				available[im.ID] = true
			}
		}
		filtered := make([]string, 0, len(images))
		for _, im := range images {
			if available[im] {
				filtered = append(filtered, im)
			}
		}
		images = filtered
		good := make(map[string]bool, len(images))
		bad := make(map[string]bool, len(images))
		rows, qerr := c.DB.QueryContext(ctx, "SELECT profile_snapshot->>'image',state,COALESCE(last_error,'') FROM deployments WHERE account_id=$1 AND profile_snapshot->>'image'=ANY($2) ORDER BY created_at DESC LIMIT 200", accountID, pq.Array(images))
		if qerr == nil {
			for rows.Next() {
				var image, state, lastError string
				if rows.Scan(&image, &state, &lastError) != nil {
					continue
				}
				if state == "PANEL_COMPLETE" || state == "READY" {
					good[image] = true
				}
				if strings.Contains(strings.ToLower(lastError), "image you selected is no longer available") {
					bad[image] = true
				}
			}
			rows.Close()
		}
		proven := make([]string, 0, len(images))
		neutral := make([]string, 0, len(images))
		for _, image := range images {
			if good[image] {
				proven = append(proven, image)
			} else if !bad[image] {
				neutral = append(neutral, image)
			}
		}
		if len(proven) > 0 {
			effective.Image = proven[rand.Intn(len(proven))]
		} else if len(neutral) > 0 {
			effective.Image = neutral[rand.Intn(len(neutral))]
		} else {
			return d, &providers.Error{Class: providers.ErrorImageUnavailable, Operation: "select_image", Message: "all configured available images are quarantined"}
		}
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
