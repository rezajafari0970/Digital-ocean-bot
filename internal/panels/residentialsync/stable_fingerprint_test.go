package residentialsync

import (
	"context"
	"testing"
)

func churnPolicy() routePolicy {
	return routePolicy{StableFingerprint: true, Explicit: true, AdsOnly: true, Harden: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Host: "127.0.0.1", Port: 1080, Tag: "residential-ads-test", Password: "fixture"}}}
}
func churnClients() ([]clientRoute, []clientRoute) {
	d := clientRoute{ID: "direct-id", Email: "direct@test", Class: "DIRECT", Effective: "DIRECT"}
	return []clientRoute{d, {ID: "res-old", Email: "old@test", Class: "RESIDENTIAL", Effective: "RESIDENTIAL"}}, []clientRoute{d, {ID: "res-new", Email: "new@test", Class: "RESIDENTIAL", Effective: "RESIDENTIAL"}}
}
func TestResidentialTurnoverDoesNotReloadUnchangedRoutes(t *testing.T) {
	for _, pool := range []bool{false, true} {
		p := churnPolicy()
		p.PoolEnabled = pool
		old, new := churnClients()
		tags := []string{"inbound-443"}
		first, err := buildSettings(baseSettings(), old, tags, p)
		if err != nil {
			t.Fatal(err)
		}
		next, err := buildSettings(first, new, tags, p)
		if err != nil {
			t.Fatal(err)
		}
		if settingsHash(first) != settingsHash(next) {
			t.Fatalf("pool=%v: residential-only turnover changed routing plan", pool)
		}
		if !pool {
			f := &fakeCore{saved: first, running: first}
			if err = applyAndVerify(context.Background(), f, first, next, "", new, tags, p); err != nil {
				t.Fatal(err)
			}
			if f.saves != 0 || f.restarts != 0 {
				t.Fatalf("turnover triggered save=%d restart=%d", f.saves, f.restarts)
			}
		}
	}
}
func TestStableFingerprintStillDetectsEffectiveChanges(t *testing.T) {
	p := churnPolicy()
	old, next := churnClients()
	tags := []string{"inbound-443"}
	first, err := buildSettings(baseSettings(), old, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	changed := func(name string, cs []clientRoute, ts []string, policy routePolicy) {
		t.Helper()
		v, e := buildSettings(first, cs, ts, policy)
		if e != nil {
			t.Fatal(e)
		}
		if settingsHash(v) == settingsHash(first) {
			t.Fatal("missed effective change:", name)
		}
	}
	next[1].Class = "DIRECT"
	next[1].Effective = "DIRECT"
	changed("direct grant", next, tags, p)
	changed("direct revoke", old[1:], tags, p)
	changed("inbound", old, []string{"other-inbound"}, p)
	q := p
	q.Proxies = append([]rp(nil), p.Proxies...)
	q.Proxies[0].Password = "rotated"
	changed("credential", old, tags, q)
	q = p
	q.SniffingBlocked = true
	changed("sniffing", old, tags, q)
	q = p
	q.Residential = false
	changed("residential disabled", old, tags, q)
}

func TestMembershipReceiptAdvancesIndependentlyOfPlan(t *testing.T) {
	old, next := churnClients()
	saved := map[string]clientRoute{}
	for _, c := range old {
		saved[c.ID] = c
	}
	if !sameRouteMembership(saved, old) {
		t.Fatal("identical receipt changed")
	}
	if sameRouteMembership(saved, next) {
		t.Fatal("new membership would not persist")
	}
	if sameRouteMembership(saved, old[:1]) {
		t.Fatal("expiry would not persist")
	}
	changed := append([]clientRoute(nil), old...)
	changed[1].Effective = "BLOCKED"
	if sameRouteMembership(saved, changed) {
		t.Fatal("effective class change lost")
	}
	if sameRouteMembership(saved, []clientRoute{old[0], old[0]}) {
		t.Fatal("duplicate accepted")
	}
}
func TestStablePlanScopeAndLegacyRollback(t *testing.T) {
	for _, tc := range []struct {
		scope string
		want  bool
	}{{"", true}, {"all", true}, {"none", false}, {" p1, p2 ", true}, {"other", false}} {
		t.Setenv("DOB_ROUTING_STABLE_PLAN_PANELS", tc.scope)
		if stablePlanPanel("p2") != tc.want {
			t.Fatal(tc)
		}
	}
	p := churnPolicy()
	p.StableFingerprint = false
	old, next := churnClients()
	tags := []string{"inbound-443"}
	a, e := buildSettings(baseSettings(), old, tags, p)
	if e != nil {
		t.Fatal(e)
	}
	b, e := buildSettings(a, next, tags, p)
	if e != nil {
		t.Fatal(e)
	}
	if settingsHash(a) == settingsHash(b) {
		t.Fatal("legacy rollback behavior lost")
	}
}
