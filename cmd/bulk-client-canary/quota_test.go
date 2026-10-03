package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQuotaClientHasOnlyTunnelOutbound(t *testing.T) {
	uri := "vless://11111111-1111-4111-8111-111111111111@203.0.113.8:443?security=reality&type=tcp&sni=example.test&pbk=public&sid=1234&fp=chrome&flow=xtls-rprx-vision"
	raw, host, err := quotaXrayConfig(uri, 18000)
	if err != nil || host != "203.0.113.8" {
		t.Fatal(host, err)
	}
	var c map[string]any
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	outs := c["outbounds"].([]any)
	if len(outs) != 1 || outs[0].(map[string]any)["protocol"] != "vless" {
		t.Fatal("direct fallback available")
	}
	in := c["inbounds"].([]any)[0].(map[string]any)
	if in["listen"] != "127.0.0.1" {
		t.Fatal("client listener exposed")
	}
	for _, bad := range []string{strings.Replace(uri, "security=reality", "security=tls", 1), strings.Replace(uri, "type=tcp", "type=ws", 1), strings.Replace(uri, "flow=xtls-rprx-vision", "flow=", 1), strings.Replace(uri, "203.0.113.8", "unverified.test", 1)} {
		if _, _, e := quotaXrayConfig(bad, 18000); e == nil {
			t.Fatal("unsupported URI accepted")
		}
	}
}
func TestQuotaCallbackRejectsUnassignedAndPrivateAddresses(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", "192.168.1.1", "::1", "example.test", "203.0.113.99"} {
		if quotaCallbackIP(host) == nil {
			t.Fatal(host)
		}
	}
}
