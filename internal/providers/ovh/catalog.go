package ovh

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
)

type flavorData struct {
	PlanCodes struct {
		Hourly  string `json:"hourly"`
		Monthly string `json:"monthly"`
		License string `json:"license"`
	} `json:"planCodes"`
	Quota     *int   `json:"quota"`
	Inbound   int    `json:"inboundBandwidth"`
	Outbound  int    `json:"outboundBandwidth"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	Type      string `json:"type"`
	OS        string `json:"osType"`
	CPU       int    `json:"vcpus"`
	RAM       int    `json:"ram"`
	Disk      int    `json:"disk"`
	Available bool   `json:"available"`
}
type imageData struct {
	Type       string `json:"type"`
	User       string `json:"user"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Region     string `json:"region"`
	Status     string `json:"status"`
	Visibility string `json:"visibility"`
	Created    string `json:"creationDate"`
	MinDisk    int    `json:"minDisk"`
	MinRAM     int    `json:"minRam"`
	FlavorType string `json:"flavorType"`
}

var officialUbuntuName = regexp.MustCompile(`^Ubuntu (22\.04|24\.04|26\.04)( LTS)?$`)

func usableImage(im imageData) bool {
	return uuidPattern.MatchString(im.ID) && regionPattern.MatchString(im.Region) && strings.EqualFold(im.Status, "active") && im.Visibility == "public" && im.Type == "linux" && im.User == "ubuntu" && officialUbuntuName.MatchString(im.Name)
}
func (d *Driver) flavors(ctx context.Context, region string) ([]flavorData, error) {
	p := d.client.path("/flavor")
	if region != "" {
		p += "?region=" + url.QueryEscape(region)
	}
	return list(ctx, d.client, p, func(x flavorData) string { return x.Region + ":" + x.ID })
}
func (d *Driver) images(ctx context.Context, region string) ([]imageData, error) {
	p := d.client.path("/image")
	if region != "" {
		p += "?region=" + url.QueryEscape(region)
	}
	return list(ctx, d.client, p, func(x imageData) string { return x.Region + ":" + x.ID })
}
func (d *Driver) CatalogCacheVersion() string { return "ovh-regional-pricing-v1" }

func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Pricing has a bounded independent deadline and never makes discovery fail.
	priced := make(chan priceEvidence, 1)
	go func() { priced <- d.pricing(ctx) }()
	rs, e := list(ctx, d.client, d.client.path("/region"), func(x string) string { return x })
	if e != nil {
		return providers.Catalog{}, e
	}
	details := make([]regionData, len(rs))
	errs := make([]error, len(rs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, id := range rs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			details[i], errs[i] = d.region(ctx, id)
		}(i, id)
	}
	wg.Wait()
	regions := []providers.Region{}
	available := map[string]bool{}
	for i, r := range details {
		if errs[i] != nil {
			return providers.Catalog{}, errs[i]
		}
		if r.supported() {
			available[r.Name] = true
			regions = append(regions, providers.Region{ID: r.Name, Name: r.Location, CountryCode: r.Country, Available: true})
		}
	}
	fs, e := d.flavors(ctx, "")
	if e != nil {
		return providers.Catalog{}, e
	}
	ims, e := d.images(ctx, "")
	if e != nil {
		return providers.Catalog{}, e
	}
	pricing := <-priced
	plans := map[string]providers.Plan{}
	for _, f := range fs {
		if f.Name == "" || !uuidPattern.MatchString(f.ID) || !regionPattern.MatchString(f.Region) {
			continue
		}
		reason := flavorSupport(f)
		supported := reason == "" && available[f.Region]
		if reason == "" && !available[f.Region] {
			reason = "Region is inactive or requires a 3-AZ/local-zone network driver"
		}
		if reason == "" && !f.Available {
			reason = "Out of stock"
		}
		p, ok := plans[f.Name]
		if !ok {
			p = providers.Plan{ID: f.Name, Name: f.Name, CPU: f.CPU, MemoryMB: f.RAM * 1024, DiskGB: f.Disk, IDsByRegion: map[string]string{}, DetailsByRegion: map[string]providers.PlanRegionDetails{}, PricesByRegion: map[string]providers.PlanPrice{}}
		}
		if _, duplicate := p.DetailsByRegion[f.Region]; duplicate {
			return providers.Catalog{}, problem(providers.ErrorUnavailable, "catalog", "DUPLICATE_REGIONAL_FLAVOR", 200)
		}
		p.DetailsByRegion[f.Region] = providers.PlanRegionDetails{NativeID: f.ID, OS: f.OS, Type: f.Type, StockAvailable: f.Available, Supported: supported, Reason: reason, Quota: f.Quota, CPU: f.CPU, MemoryMB: f.RAM * 1024, DiskGB: f.Disk, InboundMbps: f.Inbound, OutboundMbps: f.Outbound}
		if supported && f.Available {
			p.Available = true
			p.IDsByRegion[f.Region] = f.ID
			p.AvailableRegions = append(p.AvailableRegions, f.Region)
		}
		if price, known := pricing.flavor(f); known {
			p.PricesByRegion[f.Region] = price
		}
		plans[f.Name] = p
	}
	latest := map[string]map[string]imageData{}
	for _, im := range ims {
		if !available[im.Region] || !usableImage(im) || im.FlavorType != "" {
			continue
		}
		v := cloudutil.Version(im.Name)
		if latest[v] == nil {
			latest[v] = map[string]imageData{}
		}
		if prev, ok := latest[v][im.Region]; !ok || im.Created > prev.Created {
			latest[v][im.Region] = im
		}
	}
	out := providers.Catalog{Pricing: &pricing.meta, Regions: regions, Plans: []providers.Plan{}, Images: []providers.Image{}}
	for _, p := range plans {
		sort.Strings(p.AvailableRegions)
		out.Plans = append(out.Plans, p)
	}
	sort.Slice(out.Plans, func(i, j int) bool {
		if out.Plans[i].MemoryMB != out.Plans[j].MemoryMB {
			return out.Plans[i].MemoryMB < out.Plans[j].MemoryMB
		}
		return out.Plans[i].ID < out.Plans[j].ID
	})
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		if len(latest[v]) == 0 {
			continue
		}
		im := providers.Image{ID: "ovh-ubuntu-" + v, Name: "Ubuntu " + v, Family: "ubuntu", Version: v, Architecture: "x86_64", Available: true, IDsByRegion: map[string]string{}}
		for region, x := range latest[v] {
			im.IDsByRegion[region] = x.ID
		}
		out.Images = append(out.Images, im)
	}
	// OVH also lists service/legacy regions such as SBG with no compute flavors.
	// They must never become selectable launch targets.
	out.Regions = providers.CatalogRegionsWithOptions(out)
	return out, nil
}
func (d *Driver) checkQuota(ctx context.Context, region string, f flavorData) error {
	type quota struct {
		Region   string `json:"region"`
		Instance *struct {
			MaxCores      *int `json:"maxCores"`
			UsedCores     *int `json:"usedCores"`
			MaxInstances  *int `json:"maxInstances"`
			UsedInstances *int `json:"usedInstances"`
			MaxRAM        *int `json:"maxRam"`
			UsedRAM       *int `json:"usedRAM"`
		} `json:"instance"`
	}
	qs, e := list(ctx, d.client, d.client.path("/quota"), func(x quota) string { return x.Region })
	if e != nil {
		return e
	}
	for _, q := range qs {
		if q.Region != region {
			continue
		}
		if q.Instance == nil {
			return problem(providers.ErrorUnavailable, "quota", "QUOTA_EVIDENCE_MISSING", 200)
		}
		x := q.Instance
		for _, pair := range []struct {
			limit, used *int
			need        int
		}{{x.MaxCores, x.UsedCores, f.CPU}, {x.MaxInstances, x.UsedInstances, 1}, {x.MaxRAM, x.UsedRAM, f.RAM * 1024}} {
			if pair.limit == nil || pair.used == nil || *pair.used < 0 || *pair.limit < -1 {
				return problem(providers.ErrorUnavailable, "quota", "INVALID_QUOTA", 200)
			}
			if *pair.limit >= 0 && *pair.limit-*pair.used < pair.need {
				return problem(providers.ErrorRegionCapacity, "quota", "REGIONAL_QUOTA_EXCEEDED", 0)
			}
		}
		return nil
	}
	return problem(providers.ErrorUnavailable, "quota", "REGIONAL_QUOTA_MISSING", 200)
}

// The legacy local-disk contract has no durable floating-IP, gateway or AZ
// intent. Do not advertise 3-AZ/local-zone launches as ordinary 1-AZ instances.
type regionData struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Type     string `json:"type"`
	Country  string `json:"countryCode"`
	Location string `json:"datacenterLocation"`
}

func (r regionData) supported() bool { return r.Status == "UP" && r.Type == "region" }
func (d *Driver) region(ctx context.Context, id string) (regionData, error) {
	var r regionData
	if !regionPattern.MatchString(id) {
		return r, problem(providers.ErrorInvalidRequest, "region", "INVALID_REGION", 0)
	}
	if e := d.client.do(ctx, http.MethodGet, d.client.path("/region/"+id), nil, &r); e != nil {
		return r, e
	}
	if r.Name != id || r.Status == "" || r.Type == "" {
		return r, problem(providers.ErrorUnavailable, "region", "INVALID_REGION_RESPONSE", 200)
	}
	return r, nil
}
