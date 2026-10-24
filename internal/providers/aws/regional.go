package aws

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// Each region owns its network, signed client and native resource IDs. The
// original account identity remains stable for existing database ownership.
type regionalNetwork struct {
	Subnet  string   `json:"subnet_id"`
	Groups  []string `json:"security_group_ids,omitempty"`
	Managed bool     `json:"managed_network"`
}
type regionalDriver struct {
	primary   *Driver
	regions   []string
	drivers   map[string]*Driver
	resources providers.AccountResources
}
type regionalCredential struct {
	source         providers.CredentialSource
	anchor, region string
	network        regionalNetwork
}

func (s regionalCredential) Get(ctx context.Context) ([]byte, error) {
	c, e := credentials(ctx, s.source)
	if e != nil {
		return nil, e
	}
	n, ok := c.Networks[s.region]
	if s.region == c.Region {
		n = regionalNetwork{c.Subnet, c.Groups, c.ManagedNetwork}
		ok = true
	}
	if c.Region != s.anchor || !ok || !reflect.DeepEqual(n, s.network) {
		return nil, problem(providers.ErrorAuthentication, "credential", "REGION_SCOPE_CHANGED", 0)
	}
	c.Region, c.Subnet, c.Groups, c.ManagedNetwork, c.Networks = s.region, n.Subnet, n.Groups, n.Managed, nil
	return json.Marshal(c)
}
func openRegional(r providers.OpenRequest, c credential) (providers.Driver, error) {
	expectedAccount := ""
	if r.ExpectedExternalID != "" {
		p := strings.Split(r.ExpectedExternalID, ":")
		if len(p) != 2 || !accountPattern.MatchString(p[0]) || p[1] != c.Region {
			return nil, problem(providers.ErrorAuthentication, "account", "ACCOUNT_SCOPE_MISMATCH", 0)
		}
		expectedAccount = p[0]
	}
	d := &regionalDriver{drivers: map[string]*Driver{}, resources: r.Resources}
	nets := map[string]regionalNetwork{c.Region: {c.Subnet, c.Groups, c.ManagedNetwork}}
	for region, n := range c.Networks {
		nets[region] = n
	}
	for region, n := range nets {
		expected := ""
		if expectedAccount != "" {
			expected = expectedAccount + ":" + region
		}
		var resources providers.AccountResources
		if r.Resources != nil {
			resources = regionalResources{r.Resources, region}
		}
		d.drivers[region] = &Driver{client: &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: regionalCredential{r.Credentials, c.Region, region, n}, region: region, expected: expected}, accountID: r.AccountID, absence: r.Absence, resources: resources}
		d.regions = append(d.regions, region)
	}
	sort.Strings(d.regions)
	d.primary = d.drivers[c.Region]
	return d, nil
}
func (d *regionalDriver) Name() string                         { return "aws" }
func (d *regionalDriver) Capabilities() providers.Capabilities { return d.primary.Capabilities() }
func (d *regionalDriver) CatalogCacheVersion() string          { return d.primary.CatalogCacheVersion() }
func (d *regionalDriver) Account(ctx context.Context) (providers.Account, error) {
	return d.primary.Account(ctx)
}
func (d *regionalDriver) Health(ctx context.Context) error { _, e := d.Account(ctx); return e }
func (d *regionalDriver) forRef(ref string) (*Driver, error) {
	p := strings.Split(ref, ":")
	if len(p) != 2 || d.drivers[p[0]] == nil {
		return nil, problem(providers.ErrorInvalidRequest, "server", "INVALID_REGIONAL_SERVER_ID", 0)
	}
	return d.drivers[p[0]], nil
}
func (d *regionalDriver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	r, e := d.forRef(id)
	if e != nil {
		return providers.Server{}, e
	}
	return r.GetServer(ctx, id)
}
func (d *regionalDriver) DeleteServer(ctx context.Context, id string) error {
	r, e := d.forRef(id)
	if e != nil {
		return e
	}
	return r.DeleteServer(ctx, id)
}
func (d *regionalDriver) CreateServer(ctx context.Context, r providers.CreateServerRequest) (providers.CreateServerResult, error) {
	regional := d.drivers[r.RegionID]
	if regional == nil {
		return providers.CreateServerResult{Outcome: providers.OutcomeRejected, ComputeNotDispatched: true}, problem(providers.ErrorInvalidRequest, "create_server", "INVALID_REGIONAL_SERVER_ID", 0)
	}
	return regional.CreateServer(ctx, r)
}

// Bounded read fan-out never returns a partial inventory or identity search.
// Any failed region therefore cannot be mistaken for deletion/absence.
func regionalRead[T any](ctx context.Context, d *regionalDriver, fn func(context.Context, *Driver) (T, error)) ([]T, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	out := make([]T, len(d.regions))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var first sync.Once
	var failure error
	for i, region := range d.regions {
		wg.Add(1)
		go func(i int, r *Driver) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			var e error
			out[i], e = fn(ctx, r)
			if e != nil {
				first.Do(func() { failure = e; cancel() })
			}
		}(i, d.drivers[region])
	}
	wg.Wait()
	if failure != nil {
		return nil, failure
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
func (d *regionalDriver) ListServers(ctx context.Context) ([]providers.Server, error) {
	xs, e := regionalRead(ctx, d, func(ctx context.Context, r *Driver) ([]providers.Server, error) { return r.ListServers(ctx) })
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, s := range xs {
		out = append(out, s...)
	}
	return out, nil
}
func (d *regionalDriver) FindServerByIdentity(ctx context.Context, id string) ([]providers.Server, error) {
	xs, e := regionalRead(ctx, d, func(ctx context.Context, r *Driver) ([]providers.Server, error) {
		return r.FindServerByIdentity(ctx, id)
	})
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, s := range xs {
		out = append(out, s...)
	}
	return out, nil
}
func (d *regionalDriver) Inventory(ctx context.Context) (providers.Inventory, error) {
	s, e := d.ListServers(ctx)
	if e != nil {
		return providers.Inventory{}, e
	}
	return providers.Inventory{Servers: s, Resources: []providers.Resource{}, ObservedAt: time.Now().UTC()}, nil
}
func (d *regionalDriver) Capacity(ctx context.Context) (providers.Capacity, error) {
	s, e := d.ListServers(ctx)
	if e != nil {
		return providers.Capacity{}, e
	}
	return capacity(len(s)), nil
}
func (d *regionalDriver) observe(ctx context.Context, full bool) (providers.Observation, error) {
	a, e := d.Account(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	inv, e := d.Inventory(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	cat := providers.Catalog{}
	if full {
		cat, e = d.Catalog(ctx)
		if e != nil {
			return providers.Observation{}, e
		}
	}
	return providers.Observation{Account: a, Inventory: inv, Capacity: capacity(len(inv.Servers)), Catalog: cat, ObservedAt: time.Now().UTC()}, nil
}
func (d *regionalDriver) Observe(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, true)
}
func (d *regionalDriver) ObserveFast(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, false)
}
func (d *regionalDriver) Catalog(ctx context.Context) (providers.Catalog, error) {
	cats, e := regionalRead(ctx, d, func(ctx context.Context, r *Driver) (providers.Catalog, error) { return r.Catalog(ctx) })
	if e != nil {
		return providers.Catalog{}, e
	}
	out := providers.Catalog{}
	plans := map[string]providers.Plan{}
	images := map[string]providers.Image{}
	for _, cat := range cats {
		out.Regions = append(out.Regions, cat.Regions...)
		for _, p := range cat.Plans {
			old, ok := plans[p.ID]
			if !ok {
				plans[p.ID] = p
				continue
			}
			if old.CPU != p.CPU || old.MemoryMB != p.MemoryMB {
				return providers.Catalog{}, problem(providers.ErrorUnavailable, "catalog", "REGIONAL_PLAN_MISMATCH", 200)
			}
			old.AvailableRegions = append(old.AvailableRegions, p.AvailableRegions...)
			if old.PricesByRegion == nil {
				old.PricesByRegion = map[string]providers.PlanPrice{}
			}
			for r, v := range p.PricesByRegion {
				old.PricesByRegion[r] = v
			}
			plans[p.ID] = old
		}
		for _, im := range cat.Images {
			old, ok := images[im.ID]
			if !ok {
				images[im.ID] = im
				continue
			}
			for r, id := range im.IDsByRegion {
				old.IDsByRegion[r] = id
			}
			images[im.ID] = old
		}
		if cat.Pricing != nil {
			if out.Pricing == nil {
				p := *cat.Pricing
				out.Pricing = &p
			} else if cat.Pricing.Status != out.Pricing.Status {
				out.Pricing.Status = "partial"
			}
			if cat.Pricing.ObservedAt.Before(out.Pricing.ObservedAt) {
				out.Pricing.ObservedAt = cat.Pricing.ObservedAt
			}
		}
	}
	for _, p := range plans {
		out.Plans = append(out.Plans, p)
	}
	sortPlans(out.Plans)
	for _, im := range images {
		out.Images = append(out.Images, im)
	}
	sort.Slice(out.Images, func(i, j int) bool { return out.Images[i].Version > out.Images[j].Version })
	return out, nil
}

// Filter only after validating that every journal record belongs to an enabled
// credential scope. Unknown scopes stop cleanup rather than disappear silently.
type regionalResources struct {
	providers.AccountResources
	region string
}

func (r regionalResources) List(ctx context.Context) ([]providers.AccountResource, error) {
	xs, e := r.AccountResources.List(ctx)
	if e != nil {
		return nil, e
	}
	out := []providers.AccountResource{}
	for _, x := range xs {
		if strings.HasPrefix(x.Scope, r.region+":") {
			out = append(out, x)
		}
	}
	return out, nil
}
func (r regionalResources) WithLock(ctx context.Context, scope, name string, fn func(providers.AccountResourceSession) error) error {
	if !strings.HasPrefix(scope, r.region+":") {
		return problem(providers.ErrorPermissionDenied, "network_cleanup", "NETWORK_SCOPE_MISMATCH", 0)
	}
	return r.AccountResources.WithLock(ctx, scope, name, fn)
}
func (d *regionalDriver) CleanupAccountResources(ctx context.Context) error {
	if d.resources == nil {
		return problem(providers.ErrorUnavailable, "network_cleanup", "RESOURCE_JOURNAL_UNAVAILABLE", 0)
	}
	xs, e := d.resources.List(ctx)
	if e != nil {
		return e
	}
	for _, x := range xs {
		p := strings.Split(x.Scope, ":")
		if len(p) != 2 || d.drivers[p[0]] == nil || !idPattern.MatchString(p[1]) || !strings.HasPrefix(p[1], "vpc-") || x.Name != d.drivers[p[0]].networkName(p[1]) {
			return problem(providers.ErrorPermissionDenied, "network_cleanup", "NETWORK_SCOPE_MISMATCH", 0)
		}
	}
	for _, region := range d.regions {
		if e := d.drivers[region].CleanupAccountResources(ctx); e != nil {
			return e
		}
	}
	return nil
}

var _ providers.ComputeDriver = (*regionalDriver)(nil)
var _ providers.VersionedCatalogReader = (*regionalDriver)(nil)
var _ providers.FastObservationReader = (*regionalDriver)(nil)
var _ providers.AccountResourceCleaner = (*regionalDriver)(nil)
