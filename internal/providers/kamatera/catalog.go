package kamatera

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type location struct {
	ID      string `json:"id"`
	Country string `json:"name"`
	City    string `json:"subCategory"`
}
type sizeData struct {
	Suggested bool   `json:"-"`
	ID        string `json:"id"`
	RAM       int    `json:"ramMB"`
	Disk      int    `json:"diskSizeGB"`
	CPUType   string `json:"cpuType"`
	Cores     int    `json:"cpuCores"`
	Traffic   string `json:"monthlyTrafficPackage"`
}
type imageData struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Code   string         `json:"code"`
	Region string         `json:"datacenter"`
	Disk   int            `json:"osDiskSizeGB"`
	RAM    map[string]int `json:"ramMBMin"`
}

func (s sizeData) planID() string {
	return fmt.Sprintf("%d%s-%d-%d", s.Cores, s.CPUType, s.RAM, s.Disk)
}
func (s sizeData) valid() bool {
	return strings.Contains("ABTD", s.CPUType) && len(s.CPUType) == 1 && s.Cores > 0 && s.Cores <= 256 && s.RAM >= 1024 && s.Disk >= 10
}

var imagePattern = regexp.MustCompile(`^[A-Za-z0-9:._-]{1,160}$`)

func (im imageData) version() string {
	n := strings.ToLower(im.Name + " " + im.Code)
	if strings.Contains(n, "32-bit") || strings.Contains(n, "arm") || (!strings.Contains(n, "64-bit") && !strings.Contains(n, "64bit") && !strings.Contains(n, "x86_64")) {
		return ""
	}
	return cloudutil.Version(n)
}
func (d *Driver) locations(ctx context.Context) ([]location, error) {
	var xs []location
	if e := d.client.do(ctx, "GET", "/service/server?datacenter=1", nil, &xs); e != nil {
		return nil, e
	}
	if len(xs) == 0 || len(xs) > 128 {
		return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_REGIONS", 200)
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if !regionPattern.MatchString(x.ID) || seen[x.ID] {
			return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_REGIONS", 200)
		}
		seen[x.ID] = true
	}
	return xs, nil
}
func (d *Driver) sizes(ctx context.Context, region string) ([]sizeData, error) {
	var xs []sizeData
	if e := d.client.do(ctx, "GET", "/service/server?sizes=1&datacenter="+url.QueryEscape(region), nil, &xs); e != nil {
		return nil, e
	}
	if xs == nil || len(xs) > 4096 {
		return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_PLANS", 200)
	}
	return xs, nil
}
func (d *Driver) images(ctx context.Context, region string) ([]imageData, error) {
	var xs []imageData
	if e := d.client.do(ctx, "GET", "/service/server?images=1&datacenter="+url.QueryEscape(region), nil, &xs); e != nil {
		return nil, e
	}
	if xs == nil || len(xs) > 4096 {
		return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_IMAGES", 200)
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if !imagePattern.MatchString(x.ID) || !strings.HasPrefix(x.ID, region+":") || seen[x.ID] || (x.Region != "" && x.Region != region) {
			return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_IMAGE_SCOPE", 200)
		}
		seen[x.ID] = true
	}
	return xs, nil
}
func (d *Driver) CatalogCacheVersion() string { return "kamatera-cloudcli-public-pricing-v5" }
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	regions, e := d.locations(ctx)
	if e != nil {
		return providers.Catalog{}, e
	}
	priceCtx, cancelPrice := context.WithCancel(ctx)
	defer cancelPrice()
	priceResult := make(chan *publicTariff, 1)
	go func() { p, _ := d.publicTariff(priceCtx); priceResult <- p }()
	type bundle struct {
		sizes  []sizeData
		images []imageData
		err    error
	}
	bs := make([]bundle, len(regions))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i := range regions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				bs[i].err = ctx.Err()
				return
			}
			bs[i].sizes, bs[i].err = d.sizes(ctx, regions[i].ID)
			if bs[i].err == nil {
				bs[i].images, bs[i].err = d.images(ctx, regions[i].ID)
			}
			if bs[i].err == nil {
				var suggested []sizeData
				suggested, bs[i].err = d.suggestedSizes(ctx, regions[i].ID)
				bs[i].sizes = append(bs[i].sizes, suggested...)
			}
		}(i)
	}
	wg.Wait()
	// Pricing is advisory: failures cannot invalidate working API credentials.
	for _, b := range bs {
		if b.err != nil {
			return providers.Catalog{}, b.err
		}
	}
	var tariff *publicTariff
	select {
	case tariff = <-priceResult:
	case <-ctx.Done():
		return providers.Catalog{}, ctx.Err()
	}
	if ctx.Err() != nil {
		return providers.Catalog{}, ctx.Err()
	}
	priced, total := 0, 0
	out := providers.Catalog{CacheVersion: d.CatalogCacheVersion(), FetchedAt: time.Now().UTC(), Regions: []providers.Region{}, Plans: []providers.Plan{}, Images: []providers.Image{}, Pricing: &providers.CatalogPricing{Status: "unavailable", Basis: "Official public base price unavailable; retry catalog refresh", Source: pricingURL}}
	plans := map[string]*providers.Plan{}
	images := map[string]*providers.Image{}
	licenceCtx, cancelLicences := context.WithTimeout(ctx, 8*time.Second)
	defer cancelLicences()
	for i, r := range regions {
		b := bs[i]
		if b.err != nil {
			return providers.Catalog{}, b.err
		}
		out.Regions = append(out.Regions, providers.Region{ID: r.ID, Name: r.City + ", " + r.Country, CountryCode: locationCountryCode(r), Available: true})
		var nativeLicences map[string]bool
		if tariff.needsNativeLicences(r.ID, b.images) {
			nativeLicences = d.nativeImageLicences(licenceCtx, r.ID, b.images)
		}
		for _, s := range b.sizes {
			if !s.valid() || !supportedSize(s, b.images) {
				continue
			}
			id := s.planID()
			p := plans[id]
			if p == nil {
				p = &providers.Plan{ID: id, Name: id, CPU: s.Cores, MemoryMB: s.RAM, DiskGB: s.Disk, Available: true, IDsByRegion: map[string]string{}, DetailsByRegion: map[string]providers.PlanRegionDetails{}, PricesByRegion: map[string]providers.PlanPrice{}}
				plans[id] = p
			}
			if p.IDsByRegion[r.ID] != "" {
				continue
			}
			p.IDsByRegion[r.ID] = id
			origin := "provider_preset"
			if s.Suggested {
				origin = "custom_preset"
			}
			p.DetailsByRegion[r.ID] = providers.PlanRegionDetails{NativeID: s.ID, CPUType: s.CPUType, CatalogOrigin: origin, BillingCycle: "hourly", CPU: s.Cores, MemoryMB: s.RAM, DiskGB: s.Disk, Supported: true, OS: "Ubuntu"}
			total++
			if price, ok := tariff.quoteWithNativeLicences(s, r.ID, b.images, nativeLicences); ok {
				p.PricesByRegion[r.ID] = price
				priced++
			}
			p.AvailableRegions = append(p.AvailableRegions, r.ID)
		}
		sort.Slice(b.images, func(i, j int) bool { return b.images[i].ID < b.images[j].ID })
		for _, im := range b.images {
			v := im.version()
			if v == "" {
				continue
			}
			id := "ubuntu-" + v + "-x86_64"
			x := images[id]
			if x == nil {
				x = &providers.Image{ID: id, Name: "Ubuntu " + v, Family: "ubuntu", Version: v, Architecture: "x86_64", Available: true, IDsByRegion: map[string]string{}}
				images[id] = x
			}
			if x.IDsByRegion[r.ID] == "" {
				x.IDsByRegion[r.ID] = im.ID
			}
		}
	}
	if tariff != nil {
		out.Pricing.ObservedAt = tariff.observed
		out.Pricing.Currency = "USD"
		out.Pricing.Basis = "Public powered-on base estimate; hourly billing; 730-hour equivalent, not a cap; traffic, tax, discounts and Type T CPU usage excluded"
		if priced > 0 {
			out.Pricing.Status = "partial"
			if priced == total {
				out.Pricing.Status = "available"
			}
		}
	}
	for _, p := range plans {
		sort.Strings(p.AvailableRegions)
		out.Plans = append(out.Plans, *p)
	}
	for _, im := range images {
		out.Images = append(out.Images, *im)
	}
	sort.Slice(out.Regions, func(i, j int) bool { return out.Regions[i].ID < out.Regions[j].ID })
	sort.Slice(out.Plans, func(i, j int) bool {
		a, b := out.Plans[i], out.Plans[j]
		if a.MemoryMB != b.MemoryMB {
			return a.MemoryMB < b.MemoryMB
		}
		if a.CPU != b.CPU {
			return a.CPU < b.CPU
		}
		if a.DiskGB != b.DiskGB {
			return a.DiskGB < b.DiskGB
		}
		return a.ID < b.ID
	})
	sort.Slice(out.Images, func(i, j int) bool { return out.Images[i].Version > out.Images[j].Version })
	if ctx.Err() != nil {
		return providers.Catalog{}, ctx.Err()
	}
	if len(providers.CatalogRegionsWithOptions(out)) == 0 {
		return providers.Catalog{}, problem(providers.ErrorUnavailable, "catalog", "NO_SUPPORTED_UBUNTU_PLANS", 200)
	}
	return out, nil
}
func countryCode(s string) string {
	return map[string]string{"United States": "US", "Canada": "CA", "The Netherlands": "NL", "Netherlands": "NL", "Germany": "DE", "United Kingdom": "GB", "Israel": "IL", "China": "CN", "Hong Kong": "HK", "Singapore": "SG", "Japan": "JP", "Australia": "AU", "Spain": "ES", "France": "FR", "Italy": "IT", "Sweden": "SE", "Taiwan": "TW", "India": "IN"}[s]
}

func locationCountryCode(r location) string {
	if strings.Contains(strings.ToLower(r.City), "hong kong") {
		return "HK"
	}
	return countryCode(r.Country)
}
