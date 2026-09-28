package postflight

import "testing"

func TestValidateReality(t *testing.T) {
	in := Inbound{ID: 1, Remark: "k", Port: 443, Protocol: "vless", Enable: true,
		Settings:       map[string]any{"decryption": "none", "encryption": "none", "clients": []any{map[string]any{"id": "u", "flow": "xtls-rprx-vision"}}},
		StreamSettings: map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"dest": "dl.google.com:443", "serverNames": []any{"a", "b"}, "privateKey": "secret", "shortIds": []any{"04a54a16"}}},
	}
	e := Validate(in, Expected{RemoteID: 1, Remark: "k", Port: 443, UUID: "u", Flow: "xtls-rprx-vision", Target: "dl.google.com:443", ServerNames: []string{"a", "b"}, PrivateKey: "secret", ShortID: "04a54a16"})
	if e != nil {
		t.Fatal(e)
	}
}
func TestRejectsCryptoDrift(t *testing.T) {
	in := Inbound{ID: 1, Remark: "k", Port: 443, Protocol: "vless", Enable: true, Settings: map[string]any{"decryption": "none", "encryption": "auto", "clients": []any{map[string]any{"id": "u", "flow": "xtls-rprx-vision"}}}}
	if Validate(in, Expected{RemoteID: 1, Remark: "k", Port: 443}) == nil {
		t.Fatal("expected mismatch")
	}
}
