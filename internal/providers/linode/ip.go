package linode

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/netip"
	"strconv"
	"time"
)

type ipListData struct {
	Address  string `json:"address"`
	Type     string `json:"type"`
	Region   string `json:"region"`
	LinodeID int64  `json:"linode_id"`
	Public   *bool  `json:"public"`
}
type ipRangeData struct {
	Range   string  `json:"range"`
	Prefix  int     `json:"prefix"`
	Region  string  `json:"region"`
	Linodes []int64 `json:"linodes"`
}

func (d *Driver) ObserveIPs(ctx context.Context) (providers.IPObservation, error) {
	now := time.Now().UTC()
	out := providers.IPObservation{Resources: []providers.IPResource{}, Quotas: []providers.IPQuota{}, ObservedAt: now}
	if e := d.requireFullAccess(ctx); e != nil {
		return out, e
	}
	ips, e := list(ctx, d.client, "/networking/ips", func(x ipListData) string { return x.Address })
	if e != nil {
		return out, e
	}
	used4 := 0
	for _, x := range ips {
		if x.Public == nil || (x.Type != "ipv4" && x.Type != "ipv6") {
			return out, providers.IPDataError()
		}
		if !*x.Public {
			continue
		}
		a, e := netip.ParseAddr(x.Address)
		if e != nil || a.Is4() != (x.Type == "ipv4") {
			return out, providers.IPDataError()
		}
		f := 6
		kind := "slaac"
		if a.Is4() {
			f = 4
			kind = "public"
			used4++
		}
		server := ""
		if x.LinodeID > 0 {
			server = strconv.FormatInt(x.LinodeID, 10)
		}
		out.Resources = append(out.Resources, providers.IPResource{ID: a.String(), Family: f, Kind: kind, Address: a.String(), ServerID: server, Region: x.Region, Protected: true})
	}
	ranges, e := list(ctx, d.client, "/networking/ipv6/ranges", func(x ipRangeData) string { return x.Range })
	if e != nil {
		return out, e
	}
	for _, x := range ranges {
		raw := x.Range
		if _, e := netip.ParsePrefix(raw); e != nil {
			raw += "/" + strconv.Itoa(x.Prefix)
		}
		p, e := netip.ParsePrefix(raw)
		if e != nil || !p.Addr().Is6() || (p.Bits() != 56 && p.Bits() != 64) || p.Bits() != x.Prefix {
			return out, providers.IPDataError()
		}
		server := ""
		if len(x.Linodes) == 1 {
			server = strconv.FormatInt(x.Linodes[0], 10)
		}
		out.Resources = append(out.Resources, providers.IPResource{ID: p.Masked().String(), Family: 6, Kind: "routed", Prefix: p.Masked().String(), ServerID: server, Region: x.Region, Protected: true})
	}
	used6 := len(ranges)
	out.Quotas = append(out.Quotas, providers.IPQuota{Family: 4, Kind: "public", Used: &used4, Scope: "account", Source: "api:networking/ips;quota_not_exposed", ObservedAt: now}, providers.IPQuota{Family: 6, Kind: "routed", Used: &used6, Scope: "account;quota_is_region_specific", Source: "api:networking/ipv6/ranges;quota_not_exposed", ObservedAt: now})
	out.Complete = true
	return out, providers.ValidateIPObservation(out)
}
