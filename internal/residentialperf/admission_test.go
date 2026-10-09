package residentialperf

import (
	"encoding/json"
	"testing"
)

func TestAdmissionGuardRequiresKnownBlackholeSignature(t *testing.T) {
	base := AdmissionGuard{Startup: "ready", NegativeCoreAlive: true, Positive: &AdmissionObservation{Outcome: "ok", HTTPStatus: 204}, Denied: &AdmissionObservation{Outcome: "transport", CurlCode: 56}}
	if !base.Verified() {
		t.Fatal("reviewed blackhole signature rejected")
	}
	for _, change := range []func(*AdmissionGuard){func(g *AdmissionGuard) { g.NegativeCoreAlive = false }, func(g *AdmissionGuard) { g.Startup = "failed" }, func(g *AdmissionGuard) { g.Denied.HTTPStatus = 500 }, func(g *AdmissionGuard) { g.Denied.CurlCode = 35; g.Denied.Outcome = "tls" }, func(g *AdmissionGuard) { g.Denied.CurlCode = 7; g.Denied.Outcome = "connect" }, func(g *AdmissionGuard) { g.Denied.CurlCode = 97 }, func(g *AdmissionGuard) { g.Positive.HTTPStatus = 200 }} {
		b, _ := json.Marshal(base)
		var g AdmissionGuard
		json.Unmarshal(b, &g)
		change(&g)
		if g.Verified() {
			t.Fatal("unproven chain accepted")
		}
	}
}
func TestAdmissionEmptyConfigPreservesLegacyBytes(t *testing.T) {
	c := Balanced()
	before, _ := json.Marshal(c)
	c.ExcludedProxyIDs = []string{}
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Fatal("legacy empty config changed")
	}
	c.Costs = map[string]float64{"11111111-1111-4111-8111-111111111111": 1}
	c.ExcludedProxyIDs = []string{"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"}
	d := c.Clone()
	d.Costs["11111111-1111-4111-8111-111111111111"] = 2
	if c.Costs["11111111-1111-4111-8111-111111111111"] != 1 || c.ExcludedProxyIDs[0] != "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA" {
		t.Fatal("caller config mutated")
	}
}
