package vultr

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fixtureReadDriver(t *testing.T) *Driver {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/account":
			_, _ = w.Write([]byte(`{"account":{"name":"acct","email":"owner@example.test","balance":1}}`))
		case "/regions":
			_, _ = w.Write([]byte(`{"regions":[{"id":"ewr","city":"New Jersey","country":"US","continent":"North America","options":[]}]}`))
		case "/plans":
			_, _ = w.Write([]byte(`{"plans":[{"id":"vc2-1c-1gb","type":"vc2","vcpu_count":1,"ram":1024,"disk":25,"monthly_cost":5,"locations":["ewr"]}]}`))
		case "/os":
			_, _ = w.Write([]byte(`{"os":[{"id":2284,"name":"Ubuntu 24.04 LTS x64","arch":"x64","family":"ubuntu"},{"id":999,"name":"Debian 12 x64","arch":"x64","family":"debian"}]}`))
		case "/instances":
			_, _ = w.Write([]byte(`{"instances":[{"id":"i1","label":"one","main_ip":"203.0.113.8","region":"ewr","plan":"vc2-1c-1gb","status":"active","date_created":"2026-09-29T10:00:00+00:00","tags":["managed"]},{"id":"i2","label":"two","main_ip":"2001:db8::8","region":"ewr","plan":"vc2-1c-1gb","status":"active"}]}`))
		case "/instances/i1":
			_, _ = w.Write([]byte(`{"instance":{"id":"i1","label":"one","main_ip":"203.0.113.8","region":"ewr","plan":"vc2-1c-1gb","status":"active","tags":["managed"]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	c := NewClient(s.Client(), "token")
	c.base = s.URL
	return &Driver{client: c}
}
func TestReadContracts(t *testing.T) {
	d := fixtureReadDriver(t)
	ctx := context.Background()
	a, e := d.Account(ctx)
	if e != nil || a.ID == "" || a.Email != "owner@example.test" || a.Status != "active" {
		t.Fatalf("account=%+v err=%v", a, e)
	}
	cap, e := d.Capacity(ctx)
	if e != nil || cap.LimitKnown || cap.ComputeInUse != 2 || !providers.CanCreateCapacity(cap) {
		t.Fatalf("cap=%+v err=%v", cap, e)
	}
	cat, e := d.Catalog(ctx)
	if e != nil || len(cat.Regions) != 1 || len(cat.Plans) != 1 || len(cat.Images) != 2 {
		t.Fatalf("catalog=%+v err=%v", cat, e)
	}
	if cat.Plans[0].AvailableRegions[0] != "ewr" {
		t.Fatalf("plan=%+v", cat.Plans[0])
	}
	if cat.Images[0].Family != "ubuntu" || cat.Images[0].Version != "24.04" {
		t.Fatalf("image=%+v", cat.Images[0])
	}
	xs, e := d.ListServers(ctx)
	if e != nil || len(xs) != 2 || !xs[0].Ready || xs[0].PrimaryIPv4 != "203.0.113.8" || xs[1].Ready || xs[1].PrimaryIPv4 != "" {
		t.Fatalf("servers=%+v err=%v", xs, e)
	}
	obs, e := d.Observe(ctx)
	if e != nil || obs.Capacity.LimitKnown || obs.Capacity.ComputeInUse != 2 || len(obs.Inventory.Servers) != 2 {
		t.Fatalf("obs=%+v err=%v", obs, e)
	}
	fast, e := d.ObserveFast(ctx)
	if e != nil || fast.Capacity.ComputeInUse != 2 || len(fast.Inventory.Servers) != 2 || len(fast.Catalog.Plans) != 0 {
		t.Fatalf("fast=%+v err=%v", fast, e)
	}
}
func TestNormalizeUbuntuVersions(t *testing.T) {
	for _, v := range []string{"22.04", "24.04", "26.04"} {
		f, got := normalizeOS(osItem{Name: "Ubuntu " + v + " LTS x64", Family: "ubuntu"})
		if f != "ubuntu" || got != v {
			t.Fatalf("%s => %s/%s", v, f, got)
		}
	}
	f, v := normalizeOS(osItem{Name: "Debian 12", Family: "debian"})
	if f != "debian" || v != "" {
		t.Fatal(strings.Join([]string{f, v}, "/"))
	}
}

func TestObserveFastDoesNotDependOnCatalogEndpoints(t *testing.T) {
	var catalogHits int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/account":
			_, _ = w.Write([]byte("{\"account\":{\"name\":\"acct\",\"email\":\"owner@example.test\"}}"))
		case "/instances":
			_, _ = w.Write([]byte("{\"instances\":[{\"id\":\"i1\",\"main_ip\":\"203.0.113.8\",\"status\":\"active\"}]}"))
		case "/plans", "/regions", "/os":
			catalogHits++
			http.Error(w, "catalog unavailable", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	c := NewClient(s.Client(), "token")
	c.base = s.URL
	d := &Driver{client: c}
	obs, err := d.ObserveFast(context.Background())
	if err != nil {
		t.Fatalf("fast observation must survive catalog outage: %v", err)
	}
	if catalogHits != 0 {
		t.Fatalf("fast observation touched catalog endpoints %d times", catalogHits)
	}
	if obs.Capacity.ComputeInUse != 1 || len(obs.Inventory.Servers) != 1 {
		t.Fatalf("obs=%+v", obs)
	}
}
