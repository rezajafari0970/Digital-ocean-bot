package residentialsync

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"strings"
	"testing"
)

func TestDNSHasNoLocalOrDirectFallbackWhenResidentialAbsent(t *testing.T) {
	for _, configured := range []int{0, 1} {
		p := routePolicy{Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: configured}
		cs, tags := fixtureClients(t, p)
		desired, err := buildSettings(baseSettings(), cs, tags, p)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(desired["dns"])
		if strings.Contains(string(data), "localhost") || strings.Contains(string(data), "+local") {
			t.Fatal("local DNS bypass")
		}
		f := fakeCore{running: desired}
		form := url.Values{"inboundTag": {dnsTag}, "network": {"udp"}, "ip": {"1.1.1.1"}, "port": {"53"}}
		response, err := f.Do(context.Background(), routeRequest(form))
		if err != nil {
			t.Fatal(err)
		}
		var result struct{ Obj struct{ OutboundTag string } }
		json.Unmarshal(response.Body, &result)
		if result.Obj.OutboundTag != tagged(desired, blockedTag) {
			t.Fatal("DNS escaped residential", string(response.Body))
		}
	}
}
func TestStaleResidentialEffectiveDirectCannotEscape(t *testing.T) {
	p := routePolicy{Harden: true, AdsOnly: true, Residential: true, Direct: true}
	clients := []clientRoute{{ID: "old", Email: "old@test", Class: "RESIDENTIAL", Effective: "DIRECT"}}
	next, err := buildSettings(baseSettings(), clients, []string{"actual-inbound-tag"}, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"tcp", "udp"} {
		if probeRoute(t, next, "old@test", "adservice.google.com", n) != tagged(next, blockedTag) {
			t.Fatal("stale effective class leaked")
		}
	}
}
func TestHardeningCanaryScope(t *testing.T) {
	t.Setenv("DOB_RESIDENTIAL_HARDENING_PANELS", "a,b")
	if !hardeningPanel("a") || hardeningPanel("c") {
		t.Fatal("scope")
	}
	t.Setenv("DOB_RESIDENTIAL_HARDENING_PANELS", "")
	if !hardeningPanel("c") {
		t.Fatal("fleet")
	}
}

func routeRequest(form url.Values) sanaei.SessionRequest {
	return sanaei.SessionRequest{Path: "panel/api/xray/routeTest", Body: []byte(form.Encode())}
}

func TestUnobservedInboundCannotInheritDirectDefault(t *testing.T) {
	p := routePolicy{Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	cs, tags := fixtureClients(t, p)
	next, err := buildSettings(baseSettings(), cs, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	if next["outbounds"].([]any)[0].(map[string]any)["protocol"] != "blackhole" {
		t.Fatal("unsafe default outbound")
	}
	f := fakeCore{running: next}
	for _, n := range []string{"tcp", "udp"} {
		resp, e := f.Do(context.Background(), routeRequest(url.Values{"inboundTag": {"new-inbound-not-in-inventory"}, "domain": {"adservice.google.com"}, "network": {n}}))
		if e != nil {
			t.Fatal(e)
		}
		var result struct{ Obj struct{ OutboundTag string } }
		json.Unmarshal(resp.Body, &result)
		if result.Obj.OutboundTag != tagged(next, blockedTag) {
			t.Fatal("unobserved inbound escaped")
		}
	}
}
