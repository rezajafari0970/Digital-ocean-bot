package ovh

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
)

type flavorData struct {
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

// These public local-disk families are x86_64. GPU, ARM and volume-backed
// families require separate capability/cleanup contracts before enabling them.
var x86Flavor = regexp.MustCompile(`^([bcr][23]|d2|s1)-[0-9]+$`)

// The legacy /flavor schema reports RAM in GiB; image minima and quota RAM use MiB/MB.
func usableFlavor(f flavorData) bool {
	return uuidPattern.MatchString(f.ID) && regionPattern.MatchString(f.Region) && x86Flavor.MatchString(f.Name) && f.Available && f.CPU > 0 && f.RAM > 0 && f.Disk > 0 && strings.EqualFold(f.OS, "linux")
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
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	rs, e := list(ctx, d.client, d.client.path("/region"), func(x string) string { return x })
	if e != nil {
		return providers.Catalog{}, e
	}
	regions := []providers.Region{}
	available := map[string]bool{}
	for _, id := range rs {
		r, e := d.region(ctx, id)
		if e != nil {
			return providers.Catalog{}, e
		}
		if r.supported() {
			available[id] = true
			regions = append(regions, providers.Region{ID: id, Name: r.Location + " (" + id + ")", CountryCode: r.Country, Available: true})
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
	plans := map[string]providers.Plan{}
	for _, f := range fs {
		if !available[f.Region] || !usableFlavor(f) {
			continue
		}
		p, ok := plans[f.Name]
		if !ok {
			p = providers.Plan{ID: f.Name, Name: f.Name, CPU: f.CPU, MemoryMB: f.RAM * 1024, DiskGB: f.Disk, Available: true, IDsByRegion: map[string]string{}}
		} else if p.CPU != f.CPU || p.MemoryMB != f.RAM*1024 || p.DiskGB != f.Disk {
			return providers.Catalog{}, problem(providers.ErrorUnavailable, "catalog", "FLAVOR_SHAPE_MISMATCH", 200)
		}
		if p.IDsByRegion[f.Region] != "" {
			return providers.Catalog{}, problem(providers.ErrorUnavailable, "catalog", "DUPLICATE_REGIONAL_FLAVOR", 200)
		}
		p.IDsByRegion[f.Region] = f.ID
		p.AvailableRegions = append(p.AvailableRegions, f.Region)
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
	out := providers.Catalog{Regions: regions, Plans: []providers.Plan{}, Images: []providers.Image{}}
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
				return problem(providers.ErrorCapacity, "quota", "REGIONAL_QUOTA_EXCEEDED", 0)
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
