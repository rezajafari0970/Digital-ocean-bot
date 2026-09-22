package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
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
	op := droplets.BuildCreateOperation(d.AccountID, r.Profile)
	op.IdempotencyKey = "deploy:" + d.ID + ":create"
	result, err := r.Droplets.Create(ctx, op, r.Profile)
	if err != nil {
		return d, err
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
	return d, nil
}
func (r RuntimeSteps) Provision(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Provisioner == nil {
		return d, ErrRuntimeConfig
	}
	t := r.target(d)
	_, err := r.Provisioner.Execute(ctx, t, r.ProvisionPlan)
	return d, err
}
func (r RuntimeSteps) ImportDatabase(ctx context.Context, d Deployment) (Deployment, error) {
	if r.Database == nil {
		return d, ErrRuntimeConfig
	}
	return d, r.Database.Import(ctx, r.target(d), r.Template, r.DatabasePaths)
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
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(attempt) * time.Second
	if d > 10*time.Second {
		return 10 * time.Second
	}
	return d
}
