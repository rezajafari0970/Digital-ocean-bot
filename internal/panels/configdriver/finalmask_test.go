package configdriver_test

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
)

const profileMaskExample = `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","maxSplit":"0","length":"1-104","delay":"0"}},{"type":"fragment","settings":{"packets":"1-1","maxSplit":"11","length":"1-114","delay":"1"}}],"udp":[{"type":"header-custom","settings":{"client":[{"rand":16,"randRange":"0-255"}],"server":[{"rand":16,"randRange":"0-255"}]}}]}`

func TestProfileFinalMaskExportContracts(t *testing.T) {
	r := configdriver.Builtins()
	for _, cap := range r.Catalog() {
		if !cap.Public {
			continue
		}
		t.Run(cap.ID, func(t *testing.T) {
			s := configdriver.Spec{Protocol: cap.Protocol, Network: cap.Network, Security: cap.Security, Port: 19443}
			if s.Security == "reality" {
				s.SNI = "example.com"
			}
			if s.Network != "tcp" {
				s.Path = "/fixture"
				if s.Network == "grpc" {
					s.Path = "fixture"
				}
			}
			p := configdriver.ClassSettings{Count: 1, Publish: true, FinalMask: profileMaskExample, ExportFinalMask: true}
			if s.Protocol == "shadowsocks" {
				if p.Validate(s) == nil {
					t.Fatal("unsupported SS importer promised a mask")
				}
				p.ExportFinalMask = false
				if e := p.Validate(s); e != nil {
					t.Fatal("saved-only mask rejected", e)
				}
				return
			}
			if e := p.Validate(s); e != nil {
				t.Fatal(e)
			}
			base, e := configdriver.NewCredentials("profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, 1)
			if e != nil {
				t.Fatal(e)
			}
			policies := map[string]configdriver.ClassSettings{"DIRECT": p, "RESIDENTIAL": {Count: 1, Publish: true}}
			c, e := configdriver.ResizePolicies(base, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			for i, cl := range c.Clients {
				if cl.ID != base.Clients[i].ID || cl.Password != base.Clients[i].Password {
					t.Fatal("mask edit rotated client identity")
				}
				uri, e := r.URI(s, "192.0.2.1", "Fixture", c, cl)
				if e != nil {
					t.Fatal(e)
				}
				filtered, e := export.ApplyOutputOptions(uri, export.OutputOptions{IndependentProfile: true})
				if e != nil || filtered != uri {
					t.Fatal("mask lost at publication boundary", e)
				}
				if cl.Class != "DIRECT" {
					if strings.Contains(uri, "fm=") {
						t.Fatal("mask crossed routing class")
					}
					continue
				}
				u, e := url.Parse(uri)
				if e != nil {
					t.Fatal(e)
				}
				if u.Query().Has("finalmask") || u.Query().Get("fm") == "" {
					t.Fatal("not importable by v2rayNG")
				}
				var want, got any
				json.Unmarshal([]byte(p.FinalMask), &want)
				json.Unmarshal([]byte(u.Query().Get("fm")), &got)
				if !reflect.DeepEqual(want, got) {
					t.Fatal("operator JSON changed")
				}
				marked := export.WithRemark(uri, "🇩🇪 Test + label")
				mu, _ := url.Parse(marked)
				if mu.Query().Get("fm") != u.Query().Get("fm") || mu.Fragment != "🇩🇪 Test + label" {
					t.Fatal("output remark changed import contract")
				}
				if port, e := export.URIPort(uri); e != nil || port != s.Port {
					t.Fatal("port validation rejected masked URI", e)
				}
				outbound, e := r.Outbound(s, "192.0.2.1", c, cl)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(outbound["streamSettings"].(map[string]any)["finalmask"], got) {
					t.Fatal("probe tests another mask")
				}
			}
			p.ExportFinalMask = false
			policies["DIRECT"] = p
			off, e := configdriver.ResizePolicies(c, "profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, policies, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			if off.Clients[0].FinalMask != "" || off.Clients[0].ID != c.Clients[0].ID || policies["DIRECT"].FinalMask != p.FinalMask {
				t.Fatal("export toggle changed saved mask or identity")
			}
		})
	}
}

func TestProfileFinalMaskCDNExport(t *testing.T) {
	r := configdriver.Builtins()
	c, e := configdriver.NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	c.Clients[0].FinalMask = profileMaskExample
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		s := configdriver.Spec{Protocol: protocol, Network: "ws", Security: "tls", Port: 2083, Path: "/assets", CloudflareAccount: "11111111-1111-4111-8111-111111111111", CloudflareRecords: 1, CloudflareInterval: 1800, CloudflareMinLifetime: 2700, CloudflareMaxLifetime: 3600, CloudflareMaxActive: 4}
		for _, address := range []string{"104.16.0.1", "assets-owned.example.com"} {
			uri, e := r.CDNURI(s, "192.0.2.1", address, "assets-owned.example.com", "Fixture", c, c.Clients[0])
			if e != nil {
				t.Fatal(protocol, e)
			}
			u, e := url.Parse(uri)
			if e != nil {
				t.Fatal(e)
			}
			if u.Hostname() != address || u.Query().Get("sni") != "assets-owned.example.com" || u.Query().Get("fm") == "" || u.Query().Get("allowInsecure") != "false" {
				t.Fatal("CDN mapping or mask lost")
			}
			filtered, e := export.ApplyOutputOptions(uri, export.OutputOptions{IndependentProfile: true})
			if e != nil || filtered != uri {
				t.Fatal(protocol, "CDN output rejected", e)
			}
			if port, e := export.URIPort(uri); e != nil || port != 2083 {
				t.Fatal(protocol, "CDN port", e)
			}
		}
	}
}
