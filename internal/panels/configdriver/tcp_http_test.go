package configdriver

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestTCPHTTPTransportRoundTripAndValidation(t *testing.T) {
	r := Builtins()
	creds, err := NewCredentials("fixture-profile", "fixture-panel", []string{"DIRECT"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks"} {
		t.Run(protocol, func(t *testing.T) {
			spec := Spec{Protocol: protocol, Network: "tcp", Security: "none", Port: 15001, HeaderType: "http", Host: "play.googleapis.com"}
			payload, err := r.Build(spec, "192.0.2.1", "HTTP fixture", creds)
			if err != nil {
				t.Fatal(err)
			}
			stream := normalized(payload["streamSettings"]).(map[string]any)
			settings := stream["tcpSettings"].(map[string]any)
			header := settings["header"].(map[string]any)
			request := header["request"].(map[string]any)
			if header["type"] != "http" || request["path"].([]any)[0] != "/" || request["headers"].(map[string]any)["Host"].([]any)[0] != "play.googleapis.com" {
				t.Fatal("HTTP request settings lost", stream)
			}
			uri, err := r.URI(spec, "192.0.2.1", "HTTP fixture", creds, creds.Clients[0])
			if err != nil {
				t.Fatal(err)
			}
			if protocol == "vmess" {
				raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "vmess://"))
				if err != nil {
					t.Fatal(err)
				}
				var v map[string]any
				if json.Unmarshal(raw, &v) != nil || v["type"] != "http" || v["net"] != "tcp" || v["host"] != spec.Host || v["path"] != "/" {
					t.Fatal("VMess HTTP import lost", v)
				}
			} else {
				u, err := url.Parse(uri)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if protocol == "shadowsocks" {
					if q.Get("plugin") != "obfs-local;obfs=http;obfs-host=play.googleapis.com;path=/" {
						t.Fatal("SIP002 HTTP import lost", q)
					}
				} else if q.Get("headerType") != "http" || q.Get("host") != spec.Host || q.Get("path") != "/" || q.Get("type") != "tcp" {
					t.Fatal("HTTP URI import lost", q)
				}
			}
			payload["id"] = 42
			payload["tag"] = "owned-fixture"
			raw, _ := json.Marshal(payload)
			if id, err := Observe([]json.RawMessage{raw}, payload, 42, creds); err != nil || id != 42 {
				t.Fatal("native readback", id, err)
			}
			changed := spec
			changed.Host = "different.example.com"
			drift, err := r.Build(changed, "192.0.2.1", "HTTP fixture", creds)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Observe([]json.RawMessage{raw}, drift, 42, creds); err == nil {
				t.Fatal("HTTP Host drift accepted")
			}
		})
	}
	good := Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 15001, HeaderType: "http", Host: "play.googleapis.com", Path: "/test"}
	for _, tc := range []struct {
		name   string
		change func(*Spec)
	}{
		{"unknown", func(s *Spec) { s.HeaderType = "socks" }},
		{"other_transport", func(s *Spec) { s.Network = "ws" }},
		{"reality", func(s *Spec) { s.Protocol = "vless"; s.Security = "reality"; s.SNI = "example.com" }},
		{"flow", func(s *Spec) { s.Flow = "xtls-rprx-vision" }},
		{"missing_host", func(s *Spec) { s.Host = "" }},
		{"host_injection", func(s *Spec) { s.Host = "example.com;obfs=none" }},
		{"path_injection", func(s *Spec) { s.Path = "/x;obfs=none" }},
		{"header_injection", func(s *Spec) { s.Path = "/x\r\nHost: other" }},
		{"absolute_path", func(s *Spec) { s.Path = "//other.example.com" }},
		{"invalid_escape", func(s *Spec) { s.Path = "/%zz" }},
		{"query", func(s *Spec) { s.Path = "/?x=1" }},
		{"space", func(s *Spec) { s.Path = "/two words" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := good
			tc.change(&s)
			if r.Validate(s, true) == nil {
				t.Fatal("invalid HTTP header accepted")
			}
		})
	}
	legacy := good
	legacy.HeaderType = ""
	legacy.Host = ""
	legacy.Path = ""
	before, err := r.Build(legacy, "192.0.2.1", "legacy", creds)
	if err != nil {
		t.Fatal(err)
	}
	stream := normalized(before["streamSettings"]).(map[string]any)
	if _, ok := stream["tcpSettings"]; ok {
		t.Fatal("legacy TCP acquired HTTP headers")
	}
}
