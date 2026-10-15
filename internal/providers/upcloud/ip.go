package upcloud

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/netip"
	"time"
)

type managedIPData struct {
	Address   string `json:"address"`
	Family    string `json:"family"`
	Access    string `json:"access"`
	Server    string `json:"server"`
	Zone      string `json:"zone"`
	Floating  string `json:"floating"`
	Delegated string `json:"is_delegated"`
}

func (d *Driver) ObserveIPs(ctx context.Context) (providers.IPObservation, error) {
	now := time.Now().UTC()
	out := providers.IPObservation{Resources: []providers.IPResource{}, Quotas: []providers.IPQuota{}, ObservedAt: now}
	ac, e := d.account(ctx)
	if e != nil {
		return out, e
	}
	ips, e := list[managedIPData](ctx, d.client, "/ip_address", "ip_addresses", "ip_address")
	if e != nil {
		return out, normalize("observe_ips", e)
	}
	counts := map[int]int{4: 0, 6: 0}
	for _, x := range ips {
		// Utility addresses are returned by the official list endpoint but do not consume public-IP capacity.
		if x.Access != "public" && x.Access != "private" && x.Access != "utility" {
			return out, providers.IPDataError()
		}
		if x.Access != "public" {
			continue
		}
		a, e := netip.ParseAddr(x.Address)
		if e != nil {
			return out, providers.IPDataError()
		}
		f, key := 4, "IPv4"
		if a.Is6() {
			f, key = 6, "IPv6"
		}
		if x.Family != key {
			return out, providers.IPDataError()
		}
		kind := "public"
		if x.Floating == "yes" {
			kind = "floating"
		}
		if x.Delegated == "yes" {
			kind = "delegated"
		}
		counts[f]++
		out.Resources = append(out.Resources, providers.IPResource{ID: a.String(), Family: f, Kind: kind, Address: a.String(), ServerID: x.Server, Region: x.Zone, Protected: true})
	}
	for _, f := range []int{4, 6} {
		key := "public_ipv4"
		if f == 6 {
			key = "public_ipv6"
		}
		var limit *int
		if n := ac.Limits[key]; n != nil {
			v := int(*n)
			limit = &v
		}
		used := counts[f]
		out.Quotas = append(out.Quotas, providers.IPQuota{Family: f, Kind: "public_total", Limit: limit, Used: &used, Scope: "account", Source: "api:account.resource_limits." + key, ObservedAt: now})
	}
	out.Complete = true
	return out, providers.ValidateIPObservation(out)
}
