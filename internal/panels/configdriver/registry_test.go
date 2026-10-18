package configdriver

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestDriverCompositionAndOwnership(t *testing.T) {
	r := Builtins()
	credentials, e := NewCredentials("profile", "panel", []string{"DIRECT", "RESIDENTIAL"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, cap := range r.Catalog() {
		t.Run(cap.ID, func(t *testing.T) {
			s := Spec{Protocol: cap.Protocol, Network: cap.Network, Security: cap.Security, Port: 15000}
			if s.Network == "ws" || s.Network == "xhttp" {
				s.Path = "/independent"
			}
			if s.Network == "grpc" {
				s.Path = "independent"
			}
			if s.Security == "reality" {
				s.SNI = "example.com"
			}
			if s.Protocol == "vless" && s.Network == "tcp" && s.Security == "reality" {
				s.Flow = "xtls-rprx-vision"
			}
			if !cap.Public {
				if _, e = r.Build(s, "192.0.2.1", "owned", credentials); e == nil {
					t.Fatal("public plaintext accepted")
				}
				if _, e = r.Build(s, "127.0.0.1", "owned", credentials); e != nil {
					t.Fatal("private plaintext rejected", e)
				}
				return
			}
			payload, e := r.Build(s, "192.0.2.1", "owned", credentials)
			if e != nil {
				t.Fatal(e)
			}
			payload["id"] = 42
			payload["tag"] = "profile-tag"
			raw, _ := json.Marshal(payload)
			got, e := Observe([]json.RawMessage{raw}, payload, 42)
			if e != nil || got != 42 {
				t.Fatal("readback", got, e)
			}
			payload["port"] = 15001
			changed, _ := json.Marshal(payload)
			if _, e = Observe([]json.RawMessage{changed}, mapPort(t, raw), 42); e == nil {
				t.Fatal("transport drift accepted")
			}
			uri, e := r.URI(s, "192.0.2.1", "🇩🇪 Germany", credentials, credentials.Clients[0])
			if e != nil {
				t.Fatal(e)
			}
			if s.Protocol != "vmess" {
				u, e := url.Parse(uri)
				if e != nil || u.Port() != "15000" || u.Fragment != "🇩🇪 Germany" {
					t.Fatal("bad URI")
				}
			}
			if strings.Contains(uri, credentials.PrivateKey) {
				t.Fatal("private key leaked")
			}
		})
	}
	if e := r.RegisterProtocol(protocolPlugin("vless")); e == nil {
		t.Fatal("duplicate plugin registered")
	}
}
func mapPort(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	v, e := Object(raw)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestPlainTCPServerCatalogKeepsClientCompatibilityExplicit(t *testing.T) {
	r := Builtins()
	seen := map[string]bool{}
	for _, c := range r.Catalog() {
		if c.Network != "tcp" || c.Security != "none" {
			continue
		}
		seen[c.Protocol] = true
		if !c.Public {
			t.Fatal("server capability hidden", c.ID)
		}
		if (c.Protocol == "vless" || c.Protocol == "trojan") && c.ClientRequirement != "plain_tcp_compatible_client" {
			t.Fatal("client restriction hidden", c.ID)
		}
	}
	for _, p := range []string{"vless", "trojan", "vmess", "shadowsocks"} {
		if !seen[p] {
			t.Fatal("missing plain TCP", p)
		}
	}
}
