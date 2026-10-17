package linode

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"math"
	"sort"
	"strings"
)

type regionData struct {
	Country      string   `json:"country"`
	SiteType     string   `json:"site_type"`
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}
type accountRegion struct {
	Region      string   `json:"region"`
	Available   []string `json:"available"`
	Unavailable []string `json:"unavailable"`
}
type availabilityData struct {
	Region    string `json:"region"`
	Plan      string `json:"plan"`
	Available *bool  `json:"available"`
}
type priceData struct {
	ID      string   `json:"id"`
	Hourly  *float64 `json:"hourly"`
	Monthly *float64 `json:"monthly"`
}
type planData struct {
	ID           string      `json:"id"`
	Label        string      `json:"label"`
	Class        string      `json:"class"`
	CPU          int         `json:"vcpus"`
	Memory       int         `json:"memory"`
	Disk         int         `json:"disk"`
	GPUs         int         `json:"gpus"`
	Price        priceData   `json:"price"`
	RegionPrices []priceData `json:"region_prices"`
}
type imageData struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Deprecated   bool   `json:"deprecated"`
	Public       bool   `json:"is_public"`
	Status       string `json:"status"`
	Architecture string `json:"architecture"`
}

func usablePlan(p planData) bool {
	return p.CPU > 0 && p.Memory > 0 && p.Disk >= 1024 && p.GPUs == 0 && contains([]string{"nanode", "standard", "dedicated", "premium", "highmem"}, p.Class)
}
func imageModel(x imageData) providers.Image {
	version := ""
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		if x.ID == "linode/ubuntu"+v {
			version = v
		}
	}
	return providers.Image{ID: x.ID, Name: x.Label, Family: "ubuntu", Version: version, Architecture: "x86_64",
		Available: version != "" && x.Public && !x.Deprecated && x.Status == "available" && (x.Architecture == "" || x.Architecture == "x86_64")}
}
func priceModel(p priceData) (providers.PlanPrice, bool) {
	if p.Hourly == nil || *p.Hourly <= 0 || math.IsInf(*p.Hourly, 0) || math.IsNaN(*p.Hourly) {
		return providers.PlanPrice{}, false
	}
	monthly := float64(0)
	if p.Monthly != nil {
		if *p.Monthly <= 0 || math.IsInf(*p.Monthly, 0) || math.IsNaN(*p.Monthly) {
			return providers.PlanPrice{}, false
		}
		monthly = *p.Monthly
	}
	return providers.PlanPrice{Currency: "USD", Hourly: *p.Hourly, MonthlyEstimate: monthly}, true
}
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	empty := providers.Catalog{}
	regions, e := list(ctx, d.client, "/regions", func(x regionData) string { return x.ID })
	if e != nil {
		return empty, e
	}
	accountRegions, e := list(ctx, d.client, "/account/availability", func(x accountRegion) string { return x.Region })
	if e != nil {
		return empty, e
	}
	plans, e := list(ctx, d.client, "/linode/types", func(x planData) string { return x.ID })
	if e != nil {
		return empty, e
	}
	availability, e := list(ctx, d.client, "/regions/availability", func(x availabilityData) string {
		if x.Region == "" || x.Plan == "" {
			return ""
		}
		return x.Region + "/" + x.Plan
	})
	if e != nil {
		return empty, e
	}
	images, e := list(ctx, d.client, "/images", func(x imageData) string { return x.ID })
	if e != nil {
		return empty, e
	}
	out := providers.Catalog{Regions: []providers.Region{}, Plans: []providers.Plan{}, Images: []providers.Image{}}
	allowed := map[string]bool{}
	for _, r := range accountRegions {
		allowed[r.Region] = contains(r.Available, "Linodes") && !contains(r.Unavailable, "Linodes")
	}
	regionOK := map[string]bool{}
	for _, r := range regions {
		if r.SiteType == "core" && r.Status == "ok" && contains(r.Capabilities, "Linodes") && allowed[r.ID] {
			regionOK[r.ID] = true
			out.Regions = append(out.Regions, providers.Region{ID: r.ID, Name: r.Label, CountryCode: providers.RegionCountry("linode", r.ID, r.Label, r.Country), Available: true})
		}
	}
	available := map[string][]string{}
	for _, a := range availability {
		if a.Available == nil {
			return empty, problem(providers.ErrorUnavailable, "catalog", "MISSING_AVAILABILITY", 200)
		}
		if *a.Available && regionOK[a.Region] {
			available[a.Plan] = append(available[a.Plan], a.Region)
		}
	}
	for _, p := range plans {
		if !usablePlan(p) {
			continue
		}
		rs := available[p.ID]
		sort.Strings(rs)
		if len(rs) == 0 {
			continue
		}
		price, ok := priceModel(p.Price)
		if !ok {
			continue
		}
		prices := map[string]providers.PlanPrice{}
		for _, r := range rs {
			prices[r] = price
		}
		seen := map[string]bool{}
		for _, rp := range p.RegionPrices {
			if rp.ID == "" || seen[rp.ID] {
				return empty, problem(providers.ErrorUnavailable, "catalog", "INVALID_REGION_PRICE", 200)
			}
			seen[rp.ID] = true
			local, valid := priceModel(rp)
			if !valid {
				delete(prices, rp.ID)
				continue
			}
			if regionOK[rp.ID] {
				prices[rp.ID] = local
			}
		}
		usableRegions := []string{}
		for _, r := range rs {
			if _, ok := prices[r]; ok {
				usableRegions = append(usableRegions, r)
			}
		}
		if len(usableRegions) == 0 {
			continue
		}
		out.Plans = append(out.Plans, providers.Plan{ID: p.ID, Name: p.Label, CPU: p.CPU, MemoryMB: p.Memory, DiskGB: p.Disk / 1024, PriceHourly: price.Hourly, PriceMonthly: price.MonthlyEstimate, PricesByRegion: prices, Available: true, AvailableRegions: usableRegions})
	}
	for _, im := range images {
		x := imageModel(im)
		if x.Available {
			out.Images = append(out.Images, x)
		}
	}
	sort.Slice(out.Regions, func(i, j int) bool { return out.Regions[i].ID < out.Regions[j].ID })
	sort.Slice(out.Plans, func(i, j int) bool {
		a, b := out.Plans[i], out.Plans[j]
		if a.CPU != b.CPU {
			return a.CPU < b.CPU
		}
		if a.MemoryMB != b.MemoryMB {
			return a.MemoryMB < b.MemoryMB
		}
		return a.ID < b.ID
	})
	sort.Slice(out.Images, func(i, j int) bool { return strings.Compare(out.Images[i].Version, out.Images[j].Version) > 0 })
	if len(out.Regions) == 0 || len(out.Plans) == 0 || len(out.Images) == 0 {
		return empty, problem(providers.ErrorUnavailable, "catalog", "NO_COMPATIBLE_CATALOG", 200)
	}
	return out, nil
}
