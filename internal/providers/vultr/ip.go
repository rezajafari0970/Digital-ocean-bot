package vultr

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"
)

func (d *Driver) ObserveIPs(ctx context.Context) (providers.IPObservation, error) {
	now := time.Now().UTC()
	out := providers.IPObservation{Resources: []providers.IPResource{}, Quotas: []providers.IPQuota{}, ObservedAt: now}
	cursor := ""
	total := -1
	seen := map[string]bool{}
	pages := map[string]bool{}
	counts := map[int]int{4: 0, 6: 0}
	for page := 0; page < 100; page++ {
		if pages[cursor] {
			return out, providers.IPDataError()
		}
		pages[cursor] = true
		path := "/reserved-ips?per_page=100"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		var raw struct {
			IPs *[]struct {
				ID     string `json:"id"`
				Region string `json:"region"`
				Type   string `json:"ip_type"`
				Subnet string `json:"subnet"`
				Bits   *int   `json:"subnet_size"`
				Server string `json:"instance_id"`
			} `json:"reserved_ips"`
			Meta *struct {
				Total *int `json:"total"`
				Links struct {
					Next string `json:"next"`
				} `json:"links"`
			} `json:"meta"`
		}
		// A single GET; return Retry-After to the account lane instead of tight retries.
		if e := d.client.doOnce(ctx, http.MethodGet, path, nil, &raw); e != nil {
			return out, normalizeError("observe_ips", e)
		}
		if raw.IPs == nil || raw.Meta == nil || raw.Meta.Total == nil || *raw.Meta.Total < 0 || *raw.Meta.Total > 10000 {
			return out, providers.IPDataError()
		}
		if total < 0 {
			total = *raw.Meta.Total
		}
		if total != *raw.Meta.Total {
			return out, providers.IPDataError()
		}
		for _, x := range *raw.IPs {
			f := 0
			if x.Type == "v4" {
				f = 4
			}
			if x.Type == "v6" {
				f = 6
			}
			a, e := netip.ParseAddr(x.Subnet)
			if e != nil || f == 0 || x.ID == "" || seen[x.ID] || x.Region == "" {
				return out, providers.IPDataError()
			}
			if x.Bits == nil || *x.Bits < 1 {
				return out, providers.IPDataError()
			}
			p, e := a.Prefix(*x.Bits)
			if e != nil || a.Is4() != (f == 4) {
				return out, providers.IPDataError()
			}
			p = p.Masked()
			r := providers.IPResource{ID: x.ID, Family: f, Kind: "reserved", Prefix: p.String(), ServerID: x.Server, Region: x.Region, Protected: true}
			if f == 4 && *x.Bits == 32 {
				r.Address = a.String()
			}
			seen[x.ID] = true
			counts[f]++
			out.Resources = append(out.Resources, r)
		}
		next := raw.Meta.Links.Next
		if next == "" {
			if len(seen) != *raw.Meta.Total {
				return out, providers.IPDataError()
			}
			break
		}
		if len(*raw.IPs) == 0 || page == 99 {
			return out, providers.IPDataError()
		}
		u, e := url.Parse(next)
		if e != nil {
			return out, providers.IPDataError()
		}
		if u.RawQuery != "" {
			next = u.Query().Get("cursor")
			if next == "" {
				return out, providers.IPDataError()
			}
		}
		cursor = next
	}
	for _, f := range []int{4, 6} {
		used := counts[f]
		out.Quotas = append(out.Quotas, providers.IPQuota{Family: f, Kind: "reserved", Used: &used, Scope: "account", Source: "api:reserved-ips;quota_not_exposed;ipv" + strconv.Itoa(f), ObservedAt: now})
	}
	out.Complete = true
	return out, providers.ValidateIPObservation(out)
}
