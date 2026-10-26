package residentialsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/protocolprobe"
)

func TestProtocolUDPSelectionPreservesTCPAndDeny(t *testing.T) {
	p := routePolicy{PoolEnabled: true, Harden: true, AdsOnly: true, Residential: true, Configured: 3,
		Proxies: []rp{
			{ID: "one", Type: "socks5", Host: "127.0.0.1", Port: 1, Tag: "residential-ads-one", UDPRequired: true},
			{ID: "two", Type: "socks5", Host: "127.0.0.1", Port: 2, Tag: "residential-ads-two", UDPRequired: true, UDPVerified: true},
			{ID: "http", Type: "http", Host: "127.0.0.1", Port: 3, Tag: "residential-ads-http", UDPRequired: true, UDPVerified: true},
		}}
	base := map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}
	check := func(p routePolicy, expectedUDP int) map[string]any {
		t.Helper()
		plan, e := buildSettings(base, nil, []string{"in"}, p)
		if e != nil {
			t.Fatal(e)
		}
		tcp, udp := 0, 0
		for _, v := range plan["routing"].(map[string]any)["balancers"].([]any) {
			b := v.(map[string]any)
			if b["fallbackTag"] != tagged(plan, blockedTag) {
				t.Fatal("direct fallback introduced")
			}
			if b["tag"] == poolPrefix+"tcp-all" {
				tcp = len(b["selector"].([]string))
			}
			if b["tag"] == poolPrefix+"udp-all" {
				udp = len(b["selector"].([]string))
				if udp == 1 && b["selector"].([]string)[0] != tagged(plan, "residential-ads-two") {
					t.Fatal("unverified UDP candidate")
				}
			}
		}
		if tcp != 3 || udp != expectedUDP {
			t.Fatal("protocol selection coupled", tcp, udp)
		}
		return plan
	}
	first := check(p, 1)
	p.Proxies[1].UDPVerified = false
	empty := check(p, 0)
	if settingsHash(first) == settingsHash(empty) {
		t.Fatal("capability withdrawal did not change plan")
	}
	denied := false
	for _, v := range empty["routing"].(map[string]any)["rules"].([]any) {
		r := v.(map[string]any)
		if r["ruleTag"] == "dob-route-residential-ads-udp" {
			denied = r["outboundTag"] == tagged(empty, blockedTag) && r["balancerTag"] == nil
		}
	}
	if !denied {
		t.Fatal("empty UDP pool did not deny")
	}
	// Managed TLS/XUDP transports are measured through their native driver,
	// never mistaken for a plaintext SOCKS UDP_ASSOCIATE on the controller.
	relay := rp{ID: "relay", Type: "socks5", Host: "203.0.113.1", Port: 443, User: "fixture", Password: "fixture", Tag: "residential-ads-relay",
		Relay: &relayTransport{SecretRef: "fixture", Certificate: "fixture"}, UDPRequired: true}
	candidates, e := protocolCandidates([]rp{relay})
	if e != nil || len(candidates) != 1 || candidates[0].Outbound["mux"] == nil || candidates[0].Outbound["streamSettings"] == nil {
		t.Fatal("native relay driver lost", e)
	}
	if proxyHasUDP(relay) {
		t.Fatal("unmeasured relay admitted")
	}
	relay.UDPVerified = true
	if !proxyHasUDP(relay) {
		t.Fatal("verified native relay rejected")
	}
}

func TestProtocolHealthOriginExpiryAndClaimPostgres(t *testing.T) {
	db := trialTestDB(t)
	ctx := context.Background()
	t.Setenv("DOB_UDP_CAPABILITY_PANELS", "all")
	t.Setenv("DOB_RESIDENTIAL_POOL_PANELS", "all")
	sqlMustTrial(t, db, "INSERT INTO global_config_policies(policy_key,ports)VALUES('reality','[443]')")
	sqlMustTrial(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	_, panel, drop := relayDBPanel(t, db, "vultr", "203.0.113.9", false)
	svc := Service{DB: db}
	id := trialUUID()
	sqlMustTrial(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag,status,last_success_at)VALUES($1,'fixture','socks5','proxy.invalid',1080,'residential-ads-fixture','healthy',now())", id)
	policy := func() routePolicy {
		t.Helper()
		p, _, e := svc.policy(ctx, panel)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.Proxies) != 1 {
			t.Fatal("TCP candidate removed", len(p.Proxies))
		}
		return p
	}
	if proxyHasUDP(policy().Proxies[0]) {
		t.Fatal("unknown evidence admitted")
	}
	lease, e := svc.claimProtocolProbe(ctx)
	if e != nil || lease.Panel != panel {
		t.Fatal("claim", lease, e)
	}
	if _, e = svc.claimProtocolProbe(ctx); e != sql.ErrNoRows {
		t.Fatal("live claim was replaced", e)
	}
	sqlMustTrial(t, db, "UPDATE panel_protocol_probe_schedule SET lease_until=now()-interval '1 second' WHERE panel_id=$1", panel)
	replacement, e := svc.claimProtocolProbe(ctx)
	if e != nil || replacement.Token == lease.Token {
		t.Fatal("replacement", e)
	}
	if e = svc.finishProtocolProbe(ctx, lease, "", nil, false); e == nil {
		t.Fatal("stale owner finalized replacement")
	}
	_, origin, e := svc.protocolTarget(ctx, panel)
	if e != nil {
		t.Fatal(e)
	}
	p := policy()
	candidates, e := protocolCandidates(p.Proxies)
	if e != nil {
		t.Fatal(e)
	}
	good := []protocolprobe.Observation{{Driver: protocolprobe.DriverSOCKS5, ID: id, Digest: candidates[0].Digest, DNS: true, QUIC: true, Code: "ok"}}
	if e = svc.finishProtocolProbe(ctx, replacement, origin, good, true); e != nil {
		t.Fatal(e)
	}
	if !proxyHasUDP(policy().Proxies[0]) {
		t.Fatal("exact evidence not admitted")
	}
	for _, tc := range []struct{ name, change, restore string }{
		{"expired", "UPDATE panel_protocol_health SET checked_at=now()-interval '11 minutes'", "UPDATE panel_protocol_health SET checked_at=now()"},
		{"future", "UPDATE panel_protocol_health SET checked_at=now()+interval '1 minute'", "UPDATE panel_protocol_health SET checked_at=now()"},
		{"endpoint", "UPDATE residential_proxies SET port=1081", "UPDATE residential_proxies SET port=1080"},
		{"origin", "UPDATE deployments SET host='203.0.113.10'", "UPDATE deployments SET host='203.0.113.9'"},
		{"generation", "UPDATE deployments SET postinstall_generation=postinstall_generation+1", "UPDATE deployments SET postinstall_generation=postinstall_generation-1"},
		{"negative", "UPDATE panel_protocol_health SET quic_ok=false,code='datagram_unverified'", "UPDATE panel_protocol_health SET quic_ok=true,code='ok'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlMustTrial(t, db, tc.change)
			if proxyHasUDP(policy().Proxies[0]) {
				t.Fatal("stale evidence admitted")
			}
			sqlMustTrial(t, db, tc.restore)
			if !proxyHasUDP(policy().Proxies[0]) {
				t.Fatal("restored exact evidence rejected")
			}
		})
	}
	changed := p.Proxies[0]
	changed.Password = "new-secret"
	next, _ := protocolCandidates([]rp{changed})
	if next[0].Digest == candidates[0].Digest {
		t.Fatal("credential rotation kept evidence")
	}
	// A failed transport observation retains prior evidence only until its TTL;
	// it must not certify a failure/success or refresh the old timestamp.
	var before string
	if e = db.QueryRow("SELECT checked_at::text FROM panel_protocol_health WHERE panel_id=$1", panel).Scan(&before); e != nil {
		t.Fatal(e)
	}
	sqlMustTrial(t, db, "UPDATE panel_protocol_probe_schedule SET next_probe_at=now()")
	lease, e = svc.claimProtocolProbe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.finishProtocolProbe(ctx, lease, "", nil, false); e != nil {
		t.Fatal(e)
	}
	var after string
	if e = db.QueryRow("SELECT checked_at::text FROM panel_protocol_health WHERE panel_id=$1", panel).Scan(&after); e != nil || before != after {
		t.Fatal("unavailable probe refreshed evidence", e)
	}
	// Partial persistence must roll back both the delete and all inserted rows.
	sqlMustTrial(t, db, "UPDATE panel_protocol_probe_schedule SET next_probe_at=now()")
	lease, e = svc.claimProtocolProbe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.finishProtocolProbe(ctx, lease, origin, append(good, good[0]), true); e == nil {
		t.Fatal("duplicate result committed")
	}
	if !proxyHasUDP(policy().Proxies[0]) {
		t.Fatal("failed commit removed previous evidence")
	}
	sqlMustTrial(t, db, "UPDATE droplets SET expires_at=now()-interval '1 second' WHERE id=$1", drop)
	if proxyHasUDP(policy().Proxies[0]) {
		t.Fatal("expired origin admitted")
	}
}

func TestProtocolScopeAndStableEvidence(t *testing.T) {
	id := "b0000000-0000-4000-8000-000000000001"
	for _, tc := range []struct {
		scope string
		want  bool
	}{
		{"all", true}, {id, true}, {strings.ToUpper(id), true}, {"none", false}, {"", false}, {id + ",bad", false},
	} {
		t.Setenv("DOB_UDP_CAPABILITY_PANELS", tc.scope)
		if protocolEnabled(id) != tc.want {
			t.Fatal(tc.scope)
		}
	}
	p := rp{ID: id, Type: "socks5", Host: "proxy.invalid", Port: 1080, Tag: "residential-ads-fixture"}
	a, _ := protocolCandidates([]rp{p})
	p.UDPRequired = true
	p.UDPVerified = true
	b, _ := protocolCandidates([]rp{p})
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		t.Fatal("measurement changed its own identity")
	}
}
