package residentialsync

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRelaySelectionDistinctAndSticky(t *testing.T) {
	choices := []relayChoice{
		{Proxy: rp{ID: "a", Host: "1", DonorDroplet: "d1", DonorAccount: "a"}, Provider: "vultr", Retained: true},
		{Proxy: rp{ID: "b", Host: "2", DonorDroplet: "d1", DonorAccount: "b"}, Provider: "digitalocean"},
		{Proxy: rp{ID: "c", Host: "1", DonorDroplet: "d3", DonorAccount: "c"}, Provider: "linode"},
		{Proxy: rp{ID: "d", Host: "4", DonorDroplet: "d4", DonorAccount: "d"}, Provider: "digitalocean"},
		{Proxy: rp{ID: "e", Host: "5", DonorDroplet: "d5", DonorAccount: "e"}, Provider: "linode"},
		{Proxy: rp{ID: "f", Host: "6", DonorDroplet: "d6", DonorAccount: "f"}, Provider: "vultr"},
	}
	got := selectRelays(choices)
	if len(got) != 4 || got[0].ID != "a" || got[1].ID != "d" || got[2].ID != "e" || got[3].ID != "f" {
		t.Fatalf("selection: %v", got)
	}
	got = selectRelays(choices[1:])
	if len(got) != 5 {
		t.Fatal("replacement unavailable")
	}
}
func TestRelayPortAndRollbackOwnership(t *testing.T) {
	current := map[string]any{"inbounds": []any{map[string]any{"tag": "foreign", "port": 8080}, map[string]any{"tag": "api", "port": 12345}},
		"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}
	raws := []json.RawMessage{json.RawMessage(`{"id":1,"tag":"public","port":443}`)}
	got, e := chooseRelayPort(raws, current, "own")
	if e != nil || got != 80 {
		t.Fatal(got, e)
	}
	raws = append(raws, json.RawMessage(`{"id":2,"tag":"foreign2","port":"80-90"}`))
	if _, e = chooseRelayPort(raws, current, "own"); e == nil {
		t.Fatal("port conflict accepted")
	}
	current["inbounds"] = append(current["inbounds"].([]any), map[string]any{"tag": relayInletPrefix + "owned", "port": 80})
	p := pathPolicy(false)
	next, e := buildSettings(current, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	if len(next["inbounds"].([]any)) != 2 {
		t.Fatal("rollback must remove only managed inlet")
	}
}
func TestRelayHealthRejectsStaleAndDeduplicates(t *testing.T) {
	now := time.Unix(10000, 0)
	sample := func(o []relayObservation) []byte {
		b, _ := json.Marshal(map[string]any{"success": true, "obj": o})
		return b
	}
	tag := "residential-ads-relay-123"
	valid := relayObservation{Tag: tag, Alive: true, Delay: 2300, UpdatedAt: 9998}
	got, e := decodeRelayObservations(sample([]relayObservation{valid}), now)
	if e != nil || len(got) != 1 || !got[0].Alive || got[0].LastTryTime != 9998 {
		t.Fatal(got, e)
	}
	valid.UpdatedAt = 9900
	got, e = decodeRelayObservations(sample([]relayObservation{valid}), now)
	if e != nil || len(got) != 0 {
		t.Fatal("stale admitted")
	}
	valid.UpdatedAt = 9998
	valid.Delay = 3001
	got, e = decodeRelayObservations(sample([]relayObservation{valid}), now)
	if e != nil || len(got) != 1 || got[0].Alive {
		t.Fatal("slow admitted")
	}
	if _, e = decodeRelayObservations(sample([]relayObservation{valid, valid}), now); e == nil {
		t.Fatal("duplicates accepted")
	}
}
func TestRelayTransportRequiresAllowedPortAndPinnedTLS(t *testing.T) {
	c, e := newRelayCredential()
	if e != nil {
		t.Fatal(e)
	}
	p, e := managedSOCKS("00000000-0000-4000-8000-000000000011", "203.0.113.5", "secret", 8080, c)
	if e != nil {
		t.Fatal(e)
	}
	o, e := proxyOutbound(p)
	if e != nil || !ValidateRelayOutbound(o) {
		t.Fatal("generated transport invalid", e)
	}
	if _, e = managedSOCKS(p.ID, p.Host, "secret", 10000, c); e == nil {
		t.Fatal("prohibited port")
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "reality") || strings.Contains(string(raw), "PRIVATE KEY") {
		t.Fatal("wrong transport or private-key disclosure")
	}
	o["mux"].(map[string]any)["xudpProxyUDP443"] = "skip"
	if ValidateRelayOutbound(o) {
		t.Fatal("native random UDP allowed")
	}
}

func TestRelaySnapshotAllowsOnlyAdditions(t *testing.T) {
	old := rp{ID: "a", Host: "203.0.113.1", DonorDroplet: "server", DonorPlan: "proof", TransportHash: "credential"}
	other := rp{ID: "b", Host: "203.0.113.2"}
	if !relaySnapshotStillValid([]rp{old}, []rp{other, old}) {
		t.Fatal("addition invalidated existing valid snapshot")
	}
	if relaySnapshotStillValid([]rp{old}, []rp{other}) {
		t.Fatal("removed donor admitted")
	}
	for _, change := range []func(*rp){func(p *rp) { p.DonorPlan = "changed" }, func(p *rp) { p.TransportHash = "changed" }, func(p *rp) { p.Host = "203.0.113.3" }} {
		changed := old
		change(&changed)
		if relaySnapshotStillValid([]rp{old}, []rp{changed, other}) {
			t.Fatal("changed donor admitted")
		}
	}
}
