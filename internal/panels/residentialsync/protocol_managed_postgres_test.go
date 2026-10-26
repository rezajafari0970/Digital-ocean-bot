package residentialsync

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/protocolprobe"
	"testing"
)

func TestProtocolManagedEvidencePostgres(t *testing.T) {
	db := trialTestDB(t)
	ctx := context.Background()
	t.Setenv("DOB_UDP_CAPABILITY_PANELS", "all")
	sqlMustTrial(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	_, panel, _ := relayDBPanel(t, db, "vultr", "203.0.113.9", false)
	s := Service{DB: db}
	proxy := rp{ID: "relay", Type: "socks5", Host: "203.0.113.10", Port: 443, User: "fixture", Password: "fixture", Tag: "residential-ads-relay", Relay: &relayTransport{SecretRef: "fixture", Certificate: "fixture"}}
	candidates, e := protocolCandidates([]rp{proxy})
	if e != nil {
		t.Fatal(e)
	}
	_, origin, e := s.protocolTarget(ctx, panel)
	if e != nil {
		t.Fatal(e)
	}
	lease, e := s.claimProtocolProbe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	observation := protocolprobe.Observation{ID: proxy.ID, Driver: protocolprobe.DriverManagedRelay, Digest: candidates[0].Digest, QUIC: true, Code: "ok"}
	if e = s.finishProtocolProbe(ctx, lease, origin, []protocolprobe.Observation{observation}, true); e != nil {
		t.Fatal(e)
	}
	policy := routePolicy{Proxies: []rp{proxy}}
	if e = s.hydrateProtocolHealth(ctx, panel, &policy); e != nil || !proxyHasUDP(policy.Proxies[0]) {
		t.Fatal("managed evidence admission", e)
	}
	sqlMustTrial(t, db, "UPDATE panel_protocol_probe_schedule SET next_probe_at=now()")
	lease, e = s.claimProtocolProbe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	observation.Driver = protocolprobe.DriverSOCKS5
	if e = s.finishProtocolProbe(ctx, lease, origin, []protocolprobe.Observation{observation}, true); e == nil {
		t.Fatal("generic DNS requirement was bypassed")
	}
	policy = routePolicy{Proxies: []rp{proxy}}
	if e = s.hydrateProtocolHealth(ctx, panel, &policy); e != nil || !proxyHasUDP(policy.Proxies[0]) {
		t.Fatal("failed transaction lost valid managed evidence", e)
	}
	policy = routePolicy{Proxies: []rp{proxy}}
	policy.Proxies[0].Relay = nil
	if e = s.hydrateProtocolHealth(ctx, panel, &policy); e != nil || proxyHasUDP(policy.Proxies[0]) {
		t.Fatal("evidence crossed native driver", e)
	}
}
