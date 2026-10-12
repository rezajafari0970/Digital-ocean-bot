package linode

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func catalogFixture(path string) string {
	switch path {
	case "/v4/regions":
		return page(`[{"id":"us-east","label":"Newark","status":"ok","site_type":"core","capabilities":["Linodes"]},{"id":"id-cgk","label":"Jakarta","status":"ok","site_type":"core","capabilities":["Linodes"]},{"id":"outage","status":"outage","site_type":"core","capabilities":["Linodes"]},{"id":"blocked","status":"ok","site_type":"core","capabilities":["Linodes"]}]`, 1, 1, 4)
	case "/v4/account/availability":
		return page(`[{"region":"us-east","available":["Linodes"],"unavailable":[]},{"region":"id-cgk","available":["Linodes"],"unavailable":[]},{"region":"outage","available":["Linodes"],"unavailable":[]},{"region":"blocked","available":[],"unavailable":["Linodes"]}]`, 1, 1, 4)
	case "/v4/linode/types":
		return page(`[{"id":"g6-nanode-1","label":"Nanode","class":"nanode","vcpus":1,"memory":1024,"disk":25600,"price":{"hourly":0.0075,"monthly":5},"region_prices":[{"id":"id-cgk","hourly":0.009,"monthly":6}]},{"id":"gpu","class":"gpu","vcpus":8,"memory":8192,"disk":81920,"price":{"hourly":2,"monthly":1000}},{"id":"hourly","class":"standard","vcpus":2,"memory":2048,"disk":51200,"price":{"hourly":0.01,"monthly":null}}]`, 1, 1, 3)
	case "/v4/regions/availability":
		return page(`[{"region":"us-east","plan":"g6-nanode-1","available":true},{"region":"id-cgk","plan":"g6-nanode-1","available":true},{"region":"blocked","plan":"g6-nanode-1","available":true},{"region":"outage","plan":"g6-nanode-1","available":true},{"region":"us-east","plan":"hourly","available":true},{"region":"id-cgk","plan":"hourly","available":false}]`, 1, 1, 6)
	case "/v4/images":
		return page(`[{"id":"linode/ubuntu24.04","label":"Ubuntu 24.04 LTS","is_public":true,"deprecated":false,"status":"available"},{"id":"linode/ubuntu22.04","is_public":true,"deprecated":true,"status":"available"},{"id":"private/1","label":"Ubuntu 24.04","is_public":false,"status":"available"},{"id":"linode/ubuntu24.04-arm","is_public":true,"status":"available","architecture":"aarch64"}]`, 1, 1, 4)
	}
	return ""
}
func TestCatalogAvailabilityPriceAndImages(t *testing.T) {
	d := driver(t, func(r *http.Request) (*http.Response, error) {
		body := catalogFixture(r.URL.Path)
		if body == "" {
			t.Fatal(r.URL.Path)
		}
		return response(200, body), nil
	})
	cat, e := d.Catalog(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(cat.Regions) != 2 || len(cat.Plans) != 2 || len(cat.Images) != 1 || cat.Images[0].ID != "linode/ubuntu24.04" {
		t.Fatal(cat)
	}
	p := cat.Plans[0]
	if p.DiskGB != 25 || p.PriceMonthly != 5 || p.PricesByRegion["id-cgk"].MonthlyEstimate != 6 || len(p.AvailableRegions) != 2 {
		t.Fatal(p)
	}
	if cat.Plans[1].PricesByRegion["us-east"].MonthlyEstimate != 0 || len(cat.Plans[1].AvailableRegions) != 1 {
		t.Fatal(cat.Plans[1])
	}
}
func TestCatalogFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, path, old, new string }{
		{"permission", "/v4/account/availability", "", ""},
		{"missing-availability", "/v4/regions/availability", `"available":true`, `"ignored":true`},
		{"unknown-pricing", "/v4/linode/types", `"hourly":0.0075`, `"hourly":0`},
		{"no-image", "/v4/images", `"deprecated":false`, `"deprecated":true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := driver(t, func(r *http.Request) (*http.Response, error) {
				body := catalogFixture(r.URL.Path)
				if r.URL.Path == tc.path {
					if tc.name == "permission" {
						return response(403, "{}"), nil
					}
					body = strings.ReplaceAll(body, tc.old, tc.new)
				}
				return response(200, body), nil
			})
			cat, e := d.Catalog(context.Background())
			if tc.name == "unknown-pricing" {
				if e != nil || len(cat.Plans) != 1 || cat.Plans[0].ID != "hourly" {
					t.Fatal(cat, e)
				}
			} else if e == nil {
				t.Fatal("unsafe catalog accepted", cat)
			}
		})
	}
}
func TestAccountIncompleteAndServerIdentityMismatch(t *testing.T) {
	for _, body := range []string{"{}", `{"euuid":"id"}`, `{"euuid":"id","capabilities":[]}`} {
		d := driver(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		if _, e := d.Account(context.Background()); e == nil {
			t.Fatal("incomplete account accepted")
		}
	}
	d := driver(t, func(*http.Request) (*http.Response, error) { return response(200, server(2, nil)), nil })
	if _, e := d.GetServer(context.Background(), "1"); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal(e)
	}
	for _, state := range []string{"running", "offline", "booting", "deleting", "billing_suspension", "future-state"} {
		x := normalizeServer(serverData{ID: 1, Region: "us-east", Status: state, IPv4: []string{"10.0.0.1", "::1", "169.254.1.1", "224.0.0.1"}})
		if x.Ready || x.PrimaryIPv4 != "" {
			t.Fatal("nonpublic IP became ready", x)
		}
	}
}
