package configdriver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCloudflareTransportPortsAndStrictOutput(t *testing.T) {
	s := Spec{Protocol: "vless", Network: "ws", Security: "tls", Port: 2083, Path: "/assets", CloudflareAccount: "11111111-1111-4111-8111-111111111111", CloudflareRecords: 1, CloudflareInterval: 1800, CloudflareMinLifetime: 2700, CloudflareMaxLifetime: 3600, CloudflareMaxActive: 4}
	registry := Builtins()
	c, e := NewCredentials("fixture", "node", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		s.Protocol = protocol
		uri, e := registry.CDNURI(s, "192.0.2.1", "104.16.0.1", "assets-owned.example.com", "fixture", c, c.Clients[0])
		if e != nil {
			t.Fatal(e)
		}
		if protocol == "vmess" {
			b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "vmess://"))
			var v map[string]any
			json.Unmarshal(b, &v)
			if v["add"] != "104.16.0.1" || v["sni"] != "assets-owned.example.com" || v["host"] != "assets-owned.example.com" || v["allowInsecure"] != "false" {
				t.Fatal("VMess endpoint mapping")
			}
		} else {
			u, _ := url.Parse(uri)
			if u.Hostname() != "104.16.0.1" || u.Query().Get("sni") != "assets-owned.example.com" || u.Query().Get("host") != "assets-owned.example.com" || u.Query().Get("allowInsecure") != "false" {
				t.Fatal("endpoint mapping")
			}
		}
		out, e := registry.CDNOutbound(s, "192.0.2.1", "104.16.0.1", "assets-owned.example.com", c, c.Clients[0])
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), "127.0.0.1") || !strings.Contains(string(raw), `"allowInsecure":false`) {
			t.Fatal("probe bypasses CDN or TLS")
		}
	}
	for _, mutate := range []func(*Spec){func(x *Spec) { x.Network = "tcp" }, func(x *Spec) { x.Security = "reality" }, func(x *Spec) { x.Port = 10000 }, func(x *Spec) { x.Host = "foreign.example.com" }, func(x *Spec) { x.CloudflareAccount = "../escape" }, func(x *Spec) { x.CloudflareInterval = 4000 }, func(x *Spec) { x.CloudflareMaxActive = 1 }} {
		bad := s
		mutate(&bad)
		if registry.Validate(bad, true) == nil {
			t.Fatal("unsafe capability accepted", bad.Network, bad.Port)
		}
	}
	c.Clients[0].AllowInsecure = true
	if _, e = registry.CDNURI(s, "192.0.2.1", "104.16.0.1", "assets-owned.example.com", "fixture", c, c.Clients[0]); e == nil {
		t.Fatal("insecure client accepted")
	}
}

func TestCloudflareNativeOutboundSyntax(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("native Xray required")
	}
	spec := Spec{Protocol: "vless", Network: "ws", Security: "tls", Port: 2083, Path: "/assets", CloudflareAccount: "11111111-1111-4111-8111-111111111111", CloudflareRecords: 1, CloudflareInterval: 1800, CloudflareMinLifetime: 2700, CloudflareMaxLifetime: 3600, CloudflareMaxActive: 4}
	credentials, e := NewCredentials("fixture", "node", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		spec.Protocol = protocol
		out, e := Builtins().CDNOutbound(spec, "192.0.2.1", "104.16.0.1", "owned.example.com", credentials, credentials.Clients[0])
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(map[string]any{"outbounds": []any{out}})
		command := exec.Command(binary, "run", "-test", "-config", "stdin:")
		command.Stdin = bytes.NewReader(b)
		if e = command.Run(); e != nil {
			t.Fatal(protocol, "native config rejected", e)
		}
	}
}
