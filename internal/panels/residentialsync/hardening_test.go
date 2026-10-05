package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuiltinDNSTCPDirectEvenWhenResidentialAbsent(t *testing.T) {
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
		form := url.Values{"inboundTag": {dnsTag}, "network": {"tcp"}, "ip": {"1.1.1.1"}, "port": {"53"}}
		response, err := f.Do(context.Background(), routeRequest(form))
		if err != nil {
			t.Fatal(err)
		}
		var result struct{ Obj struct{ OutboundTag string } }
		json.Unmarshal(response.Body, &result)
		if result.Obj.OutboundTag != tagged(desired, directTag) {
			t.Fatal("DNS incorrectly depends on residential", string(response.Body))
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

func TestInvalidSniffingBlocksResidentialButKeepsDirectIndependent(t *testing.T) {
	p := routePolicy{Harden: true, SniffingBlocked: true, AdsOnly: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	cs, tags := fixtureClients(t, p)
	next, err := buildSettings(baseSettings(), cs, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		expected := directTag
		if c.Class == "RESIDENTIAL" {
			expected = blockedTag
			if c.Effective != "BLOCKED" {
				t.Fatal("unsafe identity published")
			}
		}
		for _, domain := range []string{"www.google.com", "adservice.google.com", ""} {
			for _, n := range []string{"tcp", "udp"} {
				if probeRoute(t, next, c.Email, domain, n) != tagged(next, expected) {
					t.Fatal("invalid sniffing escaped", c.Class, domain, n)
				}
			}
		}
	}
	if err := verifyRunning(context.Background(), &fakeCore{running: next}, next, cs, tags, p); err != nil {
		t.Fatal(err)
	}
}

type startingCore struct {
	*fakeCore
	remaining int
}

func (f *startingCore) Do(ctx context.Context, req sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	if req.Path == "panel/api/xray/routeTest" && f.remaining > 0 {
		f.remaining--
		return sanaei.SessionResponse{StatusCode: 200, Body: []byte(`{"success":false,"msg":"Something went wrong (rpc error: code = Unavailable desc = connection refused)"}`)}, nil
	}
	return f.fakeCore.Do(ctx, req)
}
func TestRunningProcessBeforeRouteAPIIsReadyDoesNotRestartAgain(t *testing.T) {
	p := routePolicy{Harden: true, AdsOnly: true, Residential: true, Direct: true}
	cs, tags := fixtureClients(t, p)
	desired, err := buildSettings(baseSettings(), cs, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	f := &startingCore{fakeCore: &fakeCore{saved: desired, running: desired}, remaining: 2}
	if err = applyAndVerify(context.Background(), f, desired, desired, "", cs, tags, p); err != nil {
		t.Fatal(err)
	}
	if f.saves != 0 || f.restarts != 0 {
		t.Fatal("readiness caused duplicate mutation")
	}
	f.remaining = 100
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err = verifyWhenReady(ctx, f, desired, cs, tags, p); !errors.Is(err, errRouteAPIStarting) {
		t.Fatal("not fail closed", err)
	}
}
