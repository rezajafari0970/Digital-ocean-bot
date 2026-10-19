package residentialsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestGoogleAdsSelectiveScopeAndRollback(t *testing.T) {
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	t.Setenv("DOB_RESIDENTIAL_ALLOWLIST_PANELS", "all")
	t.Setenv("DOB_RESIDENTIAL_CATEGORIES_PANELS", "all")
	for _, scope := range []string{"", "none", "invalid", a, "all"} {
		t.Setenv("DOB_RESIDENTIAL_GOOGLE_ADS_SELECTIVE_PANELS", scope)
		for _, panel := range []string{a, b} {
			want := scope == "all" || scope == panel
			if GoogleAdsSelectiveEnabled(panel) != want || StrictAllowlistEnabled(panel) == want || ExpandedCategoriesEnabled(panel) == want {
				t.Fatal(scope, panel)
			}
			if want && !reflect.DeepEqual(AllowedResidentialDomains(panel), []string{"geosite:google@ads"}) {
				t.Fatal("scope widened")
			}
		}
	}
	legacy := routePolicy{StrictAllowlist: true, ExpandedCategories: true, Residential: true, Direct: true, PoolEnabled: true, StableFingerprint: true, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test", Host: "127.0.0.1", Port: 1}}}
	cs, tags := fixtureClients(t, legacy)
	before, e := buildSettings(baseSettings(), cs, tags, legacy)
	if e != nil {
		t.Fatal(e)
	}
	selective := legacy
	selective.GoogleAdsSelective = true
	after, e := buildSettings(before, cs, tags, selective)
	if e != nil {
		t.Fatal(e)
	}
	restored, e := buildSettings(after, cs, tags, legacy)
	if e != nil {
		t.Fatal(e)
	}
	if settingsHash(before) != settingsHash(restored) || settingsHash(before) == settingsHash(after) {
		t.Fatal("exact rollback differs")
	}
}

func TestGoogleAdsSelectiveRoutingAndProxyOutage(t *testing.T) {
	for _, kind := range []string{"socks5", "http", "down", "sniffing-blocked"} {
		t.Run(kind, func(t *testing.T) {
			p := routePolicy{GoogleAdsSelective: true, StrictAllowlist: true, ExpandedCategories: true, Residential: true, Direct: true, Configured: 1, SniffingBlocked: kind == "sniffing-blocked"}
			if kind == "socks5" || kind == "http" {
				p.Proxies = []rp{{Type: kind, Tag: "residential-ads-test", Host: "127.0.0.1", Port: 1080}}
			}
			cs, tags := fixtureClients(t, p)
			s, e := buildSettings(baseSettings(), cs, tags, p)
			if e != nil {
				t.Fatal(e)
			}
			for _, c := range cs {
				if c.Class == "RESIDENTIAL" && c.Effective != "RESIDENTIAL" {
					t.Fatal("ordinary traffic disabled with ads", c)
				}
				for _, network := range []string{"tcp", "udp"} {
					for _, host := range []string{"www.google.com", "browserleaks.com", "tls.browserleaks.com", "www.youtube.com", "firebase.google.com", "fonts.googleapis.com", "scontent.xx.fbcdn.net", "www.example.com", "adservice.google.com.evil.test", "1.1.1.1"} {
						if got := probeRoute(t, s, c.Email, host, network); got != tagged(s, directTag) {
							t.Fatal("non-ad not direct", host, network, got)
						}
					}
					want := directTag
					if c.Class == "RESIDENTIAL" {
						want = blockedTag
						if len(p.Proxies) > 0 && (kind == "socks5" || network == "tcp") {
							want = p.Proxies[0].Tag
						}
					}
					if got := probeRoute(t, s, c.Email, "adservice.google.com", network); got != tagged(s, want) {
						t.Fatal("ads route wrong", kind, network, c, got)
					}
				}
			}
			f := &fakeCore{running: s}
			if e := verifyRunning(context.Background(), f, s, cs, tags, p); e != nil {
				t.Fatal("native proof expectations", e)
			}
		})
	}
}

func TestInstalledCoreGoogleAdsSelectiveTrafficAndOutage(t *testing.T) {
	if os.Getenv("XRAY_TEST_BINARY") == "" {
		t.Skip("installed core required")
	}
	for _, pool := range []bool{false, true} {
		t.Run(fmt.Sprintf("pool=%t", pool), func(t *testing.T) {
			upstream := newPoolFixture(t, "residential", 0, nil)
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "direct") }))
			defer sink.Close()
			_, pt, _ := net.SplitHostPort(sink.Listener.Addr().String())
			port, _ := strconv.Atoi(pt)
			udp, e := net.ListenPacket("udp4", net.JoinHostPort("127.0.0.1", pt))
			if e != nil {
				t.Fatal(e)
			}
			defer udp.Close()
			go func() {
				b := make([]byte, 100)
				for {
					n, a, e := udp.ReadFrom(b)
					if e != nil {
						return
					}
					udp.WriteTo(b[:n], a)
				}
			}()
			p := routePolicy{GoogleAdsSelective: true, StrictAllowlist: true, ExpandedCategories: true, PoolEnabled: pool, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Host: "127.0.0.1", Port: upstream.listener.Addr().(*net.TCPAddr).Port, Tag: "residential-ads-fixture"}}}
			s := pathConfig(t, p)
			for _, value := range s["outbounds"].([]any) {
				o := value.(map[string]any)
				if o["protocol"] == "freedom" {
					o["settings"] = map[string]any{"redirect": net.JoinHostPort("127.0.0.1", pt), "finalRules": []any{map[string]any{"action": "allow"}}}
				}
			}
			if pool {
				ping := s["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)
				ping["destination"] = "http://connectivitycheck.gstatic.com/probe"
				ping["interval"] = "1s"
				ping["timeout"] = "500ms"
			}
			address := runPathCore(t, s)
			if pool {
				poolWait(t, "observer", func() bool { return upstream.checks.Load() > 0 })
			}
			for _, host := range []string{"adservice.google.com", "pagead2.googlesyndication.com", "googleads.g.doubleclick.net"} {
				poolWait(t, "ads residential", func() bool {
					body, e := strictVLESSRequest(address, pathResID, host, host, 443)
					return e == nil && body == "residential"
				})
				body, e := strictVLESSRequest(address, pathDirectID, host, host, 443)
				if e != nil || body != "direct" {
					t.Fatal("direct class changed", body, e)
				}
			}
			hosts := []string{"www.google.com", "browserleaks.com", "www.youtube.com", "firebase.google.com", "fonts.googleapis.com", "scontent.xx.fbcdn.net", "www.example.com", "adservice.google.com.evil.test", "1.1.1.1"}
			assertOrdinary := func() {
				t.Helper()
				for _, host := range hosts {
					body, e := strictVLESSRequest(address, pathResID, host, host, port)
					if e != nil || body != "direct" {
						t.Fatal("ordinary TCP", host, body, e)
					}
					body, e = strictVLESSUDP(address, pathResID, host, port)
					if e != nil || body != "allowlist-udp" {
						t.Fatal("ordinary UDP", host, body, e)
					}
				}
			}
			assertOrdinary()
			before := upstream.tcpHits.Load()
			if body, e := strictVLESSRequest(address, pathResID, "127.0.0.1", "adservice.google.com", port); e != nil || body != "residential" {
				t.Fatal("sniffed ads", body, e)
			}
			if upstream.tcpHits.Load() <= before {
				t.Fatal("sniffed ads did not reach proxy")
			}
			upstream.down.Store(true)
			if pool {
				time.Sleep(1500 * time.Millisecond)
			}
			if body, e := strictVLESSRequest(address, pathResID, "adservice.google.com", "adservice.google.com", 443); e == nil || body == "direct" {
				t.Fatal("ads leaked during outage", body, e)
			}
			assertOrdinary()
			// The encoded public scope remains exactly the original Google Ads category.
			for _, v := range s["routing"].(map[string]any)["rules"].([]any) {
				r := v.(map[string]any)
				tag, _ := r["ruleTag"].(string)
				if tag == "dob-route-residential-ads" || tag == "dob-route-residential-ads-tcp" || tag == "dob-route-residential-ads-udp" {
					b, _ := json.Marshal(r["domain"])
					if string(b) != "[\"geosite:google@ads\"]" {
						t.Fatal("expanded scope retained")
					}
				}
			}
		})
	}
}
