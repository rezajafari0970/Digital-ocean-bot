package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
	"strconv"
	"time"
)

var ErrRuntimeConfig = errors.New("workflow runtime configuration missing")

type DropletCreator interface {
	Create(context.Context, jobs.Operation, droplets.Profile) (jobs.Operation, error)
}
type ResourceWaiter interface {
	Wait(context.Context, string) (ResourceInfo, error)
}
type Provisioner interface {
	Execute(context.Context, provisioning.Target, provisioning.Plan) (provisioning.Run, error)
}
type DatabaseImporter interface {
	Import(context.Context, provisioning.Target, sanaei.DatabaseTemplate, sanaei.DatabasePaths) error
}
type PanelConfigurer interface {
	Configure(context.Context, Deployment) error
}
type ClientRegistrar interface {
	CreateMany(context.Context, string, string, int, int, string) ([]sanaei.ClientRecord, error)
}
type TrafficRegistrar interface {
	Register(context.Context, []sanaei.ClientRecord) error
}

type ResourceInfo struct {
	ProviderID string
	Host       string
}

type RuntimeSteps struct {
	DB            *sql.DB
	Droplets      DropletCreator
	Waiter        ResourceWaiter
	Provisioner   Provisioner
	Database      DatabaseImporter
	Panel         PanelConfigurer
	Clients       ClientRegistrar
	Traffic       TrafficRegistrar
	Profile       droplets.Profile
	ProvisionPlan provisioning.Plan
	Target        provisioning.Target
	Template      sanaei.DatabaseTemplate
	DatabasePaths sanaei.DatabasePaths
}

func (r RuntimeSteps) Create(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Droplets == nil {
		return d, ErrRuntimeConfig
	}
	r.Profile.IdentityTag = "dob-deployment-" + d.ID
	op := droplets.BuildCreateOperation(d.AccountID, r.Profile)
	op.IdempotencyKey = "deploy:" + d.ID + ":create"
	regions := append([]string(nil), r.Profile.Regions...)
	if len(regions) == 0 {
		regions = []string{r.Profile.Region}
	}
	var result jobs.Operation
	var err error
	for i, region := range regions {
		p := r.Profile
		p.Region = region
		regionOp := op
		regionOp.IdempotencyKey = op.IdempotencyKey + ":region:" + region
		result, err = r.Droplets.Create(ctx, regionOp, p)
		if err == nil && result.ResourceID != "" {
			r.Profile.Region = region
			break
		}
		if err == nil {
			return d, droplets.ErrOutcomeStillUnknown
		}
		class := digitalocean.ClassifyError(err)
		if class != resilience.Permanent || !digitalocean.IsCapacityError(err) || i == len(regions)-1 {
			return d, err
		}
	}
	d.ProviderID = result.ResourceID
	return d, nil
}
func (r RuntimeSteps) WaitResource(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Waiter == nil || d.ProviderID == "" {
		return d, ErrRuntimeConfig
	}
	info, err := r.Waiter.Wait(ctx, d.ProviderID)
	if err != nil {
		return d, err
	}
	d.ProviderID = info.ProviderID
	r.Target.Host = info.Host
	if r.DB != nil && d.DropletID == "" {
		// Recovery may reach wait_resource after the droplet row was already
		// persisted. Reuse it by provider identity instead of attempting a
		// duplicate INSERT on every recovery cycle.
		err = r.DB.QueryRowContext(ctx, `SELECT id::text FROM droplets WHERE account_id=$1 AND provider_resource_id=$2 AND state<>'DELETED' ORDER BY created_at DESC LIMIT 1`, d.AccountID, d.ProviderID).Scan(&d.DropletID)
		if errors.Is(err, sql.ErrNoRows) {
			profileRaw, _ := json.Marshal(r.Profile)
			err = r.DB.QueryRowContext(ctx, `INSERT INTO droplets(id,account_id,profile_id,provider_resource_id,state,profile) VALUES(gen_random_uuid(),$1,$2,$3,'PROVISIONING',$4) RETURNING id::text`, d.AccountID, d.ProfileID, d.ProviderID, profileRaw).Scan(&d.DropletID)
		}
		if err != nil {
			return d, err
		}
	}
	return d, nil
}
func (r RuntimeSteps) Provision(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Provisioner == nil {
		return d, ErrRuntimeConfig
	}
	t := r.target(d)
	// On recovery wait_resource may already be checkpointed, so the in-memory
	// Host from that step no longer exists. Rehydrate it from the provider.
	if t.Host == "" && r.Waiter != nil && d.ProviderID != "" {
		info, err := r.Waiter.Wait(ctx, d.ProviderID)
		if err != nil {
			return d, err
		}
		t.Host = info.Host
	}
	if t.Host == "" {
		return d, ErrRuntimeConfig
	}
	_, err := r.Provisioner.Execute(ctx, t, r.ProvisionPlan)
	return d, err
}
func (r RuntimeSteps) ImportDatabase(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Database == nil {
		return d, ErrRuntimeConfig
	}
	t := r.target(d)
	if t.Host == "" && r.Waiter != nil && d.ProviderID != "" {
		info, err := r.Waiter.Wait(ctx, d.ProviderID)
		if err != nil {
			return d, err
		}
		t.Host = info.Host
	}
	if t.Host == "" {
		return d, ErrRuntimeConfig
	}
	return d, r.Database.Import(ctx, t, r.Template, r.DatabasePaths)
}
func (r RuntimeSteps) ConfigurePanel(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Panel == nil {
		return d, nil
	}
	return d, r.Panel.Configure(ctx, d)
}

func (r RuntimeSteps) RegisterClients(ctx context.Context, d Deployment, req Request) (Deployment, error) {
	if r.Clients == nil {
		return d, ErrRuntimeConfig
	}
	records, err := r.Clients.CreateMany(ctx, d.AccountID, d.DropletID, req.InboundID, req.ClientCount, req.EmailPrefix)
	if err != nil {
		return d, err
	}
	registeredClients.Store(d.ID, records)
	return d, nil
}
func (r RuntimeSteps) RegisterTraffic(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Traffic == nil {
		return d, nil
	}
	v, _ := registeredClients.Load(d.ID)
	records, _ := v.([]sanaei.ClientRecord)
	return d, r.Traffic.Register(ctx, records)
}
func (r RuntimeSteps) target(d Deployment) provisioning.Target {
	t := r.Target
	t.AccountID = d.AccountID
	t.DropletID = d.DropletID
	return t
}

var registeredClients clientCache

type clientCache struct {
	m map[string][]sanaei.ClientRecord
}

func (c *clientCache) Store(k string, v []sanaei.ClientRecord) {
	if c.m == nil {
		c.m = map[string][]sanaei.ClientRecord{}
	}
	c.m[k] = v
}
func (c *clientCache) Load(k string) (any, bool) { v, ok := c.m[k]; return v, ok }

func ProviderID(id int) string { return strconv.Itoa(id) }
func waitDelay(attempt int) time.Duration {
	// Provider polling budget: 2s, 4s, 8s, then at most once every 15s.
	// This keeps activation responsive without hammering the provider API.
	if attempt < 1 {
		attempt = 1
	}
	d := 2 * time.Second
	for i := 1; i < attempt && d < 15*time.Second; i++ {
		d *= 2
	}
	if d > 15*time.Second {
		d = 15 * time.Second
	}
	return d
}
