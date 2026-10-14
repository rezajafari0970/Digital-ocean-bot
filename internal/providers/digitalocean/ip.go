package digitalocean

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/netip"
	"strconv"
	"time"
)

type reservedIPData struct {
	RegionSlug string `json:"region_slug"`
	IP         string `json:"ip"`
	Droplet    *struct {
		ID int64 `json:"id"`
	} `json:"droplet"`
	Region struct {
		Slug string `json:"slug"`
	} `json:"region"`
}

func (d *Driver) ObserveIPs(ctx context.Context) (providers.IPObservation, error) {
	now := time.Now().UTC()
	out := providers.IPObservation{Resources: []providers.IPResource{}, Quotas: []providers.IPQuota{}, ObservedAt: now}
	var ac struct {
		Account *struct {
			Limit *int `json:"floating_ip_limit"`
		} `json:"account"`
	}
	if e := d.client.get(ctx, "account", &ac); e != nil {
		return out, normalizeError("observe_ips", e)
	}
	if ac.Account == nil {
		return out, providers.IPDataError()
	}
	for _, f := range []int{4, 6} {
		path, field := "reserved_ips", "reserved_ips"
		if f == 6 {
			path, field = "reserved_ipv6", "reserved_ipv6s"
		}
		// Only local page numbers are requested; never follow provider-controlled paths.
		total := -1
		seen := map[string]bool{}
		for page := 1; page <= 100; page++ {
			var raw struct {
				V4   *[]reservedIPData `json:"reserved_ips"`
				V6   *[]reservedIPData `json:"reserved_ipv6s"`
				Meta *struct {
					Total *int `json:"total"`
				} `json:"meta"`
			}
			if e := d.client.get(ctx, fmt.Sprintf("%s?page=%d&per_page=200", path, page), &raw); e != nil {
				return out, normalizeError("observe_ips", e)
			}
			items := raw.V4
			if field == "reserved_ipv6s" {
				items = raw.V6
			}
			if items == nil || raw.Meta == nil || raw.Meta.Total == nil || *raw.Meta.Total < 0 || *raw.Meta.Total > 20000 {
				return out, providers.IPDataError()
			}
			if total < 0 {
				total = *raw.Meta.Total
			}
			if total != *raw.Meta.Total {
				return out, providers.IPDataError()
			}
			for _, x := range *items {
				if f == 6 {
					x.Region.Slug = x.RegionSlug
				}
				a, e := netip.ParseAddr(x.IP)
				if e != nil || a.Is4() != (f == 4) || seen[a.String()] || x.Region.Slug == "" {
					return out, providers.IPDataError()
				}
				seen[a.String()] = true
				server := ""
				if x.Droplet != nil {
					if x.Droplet.ID <= 0 {
						return out, providers.IPDataError()
					}
					server = strconv.FormatInt(x.Droplet.ID, 10)
				}
				out.Resources = append(out.Resources, providers.IPResource{ID: a.String(), Address: a.String(), Family: f, Kind: "reserved", ServerID: server, Region: x.Region.Slug, Protected: true})
			}
			if len(seen) == total {
				break
			}
			if len(*items) == 0 || len(seen) > total || page == 100 {
				return out, providers.IPDataError()
			}
		}
		used := len(seen)
		var limit *int
		source := "api:reserved_ipv6;quota_not_exposed"
		if f == 4 {
			limit = ac.Account.Limit
			source = "api:account.floating_ip_limit"
		}
		out.Quotas = append(out.Quotas, providers.IPQuota{Family: f, Kind: "reserved", Limit: limit, Used: &used, Scope: "account", Source: source, ObservedAt: now})
	}
	out.Complete = true
	return out, providers.ValidateIPObservation(out)
}
