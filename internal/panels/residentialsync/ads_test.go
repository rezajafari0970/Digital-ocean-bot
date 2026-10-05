package residentialsync

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"strings"
	"testing"
)

func probeRoute(t *testing.T, setting map[string]any, email, domain, network string) string {
	t.Helper()
	f := fakeCore{running: setting}
	form := url.Values{"inboundTag": {"actual-inbound-tag"}, "email": {email}, "domain": {domain}, "network": {network}}
	resp, err := f.Do(context.Background(), sanaei.SessionRequest{Path: "panel/api/xray/routeTest", Body: []byte(form.Encode())})
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Obj struct{ OutboundTag string } }
	if err = json.Unmarshal(resp.Body, &v); err != nil {
		t.Fatal(err)
	}
	return v.Obj.OutboundTag
}
func TestAdsOnlyTCPUDPRoutingAndHTTPProtectedUDPDenied(t *testing.T) {
	for _, kind := range []string{"socks5", "http", "down", "absent"} {
		t.Run(kind, func(t *testing.T) {
			p := routePolicy{AdsOnly: true, Harden: true, Residential: true, Direct: true, Configured: 1}
			if kind == "absent" {
				p.Configured = 0
			}
			if kind == "http" || kind == "socks5" {
				p.Proxies = []rp{{Type: kind, Tag: "residential-ads-test", Host: "example.test", Port: 1080}}
			}
			cs, tags := fixtureClients(t, p)
			desired, err := buildSettings(baseSettings(), cs, tags, p)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range cs {
				for _, network := range []string{"tcp", "udp"} {
					for _, domain := range []string{"www.google.com", "www.facebook.com", "pixel.facebook.com", "api.ipify.org", "notbrowserleaks.com", "browserleaks.com.example.org", "", "1.1.1.1", "2001:db8::1"} {
						got := probeRoute(t, desired, c.Email, domain, network)
						want := directTag
						if got != tagged(desired, want) {
							t.Fatalf("non-ad not direct: %s %s %s", c.Class, domain, network)
						}
					}
					base := directTag
					if c.Class == "RESIDENTIAL" {
						base = blockedTag
						if len(p.Proxies) > 0 && (network == "tcp" || kind == "socks5") {
							base = p.Proxies[0].Tag
						}
					}
					for _, domain := range []string{"adservice.google.com", "browserleaks.com", "tls.browserleaks.com", "a.b.browserleaks.com"} {
						if got := probeRoute(t, desired, c.Email, domain, network); got != tagged(desired, base) {
							t.Fatalf("ad route %s %s %s: %s", kind, c.Class, network, got)
						}
					}
				}
			}
			// HTTP UDP denial is domain-scoped; ordinary UDP remains direct.
			for _, v := range desired["routing"].(map[string]any)["rules"].([]any) {
				r := v.(map[string]any)
				if r["ruleTag"] == "dob-route-residential-udp" && r["domain"] == nil {
					t.Fatal("overbroad residential UDP block")
				}
			}
		})
	}
}
func TestManagedAdsOverrideLegacyAdBlockWithoutTouchingOtherInbound(t *testing.T) {
	p := routePolicy{AdsOnly: true, Harden: true, Residential: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	cs, tags := fixtureClients(t, p)
	base := baseSettings()
	routing := base["routing"].(map[string]any)
	routing["rules"] = append(routing["rules"].([]any), map[string]any{"type": "field", "domain": []string{"geosite:category-ads-all"}, "outboundTag": "blocked"})
	next, err := buildSettings(base, cs, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := probeRoute(t, next, cs[0].Email, "adservice.google.com", "tcp"); !strings.HasPrefix(got, "residential-ads-") {
		t.Fatal("legacy ad block won", got)
	}
	rules := next["routing"].(map[string]any)["rules"].([]any)
	if rules[len(rules)-1].(map[string]any)["outboundTag"] != "blocked" {
		t.Fatal("legacy rule removed")
	}
}
