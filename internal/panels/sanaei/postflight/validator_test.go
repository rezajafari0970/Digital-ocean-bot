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

func TestValidateRealityWithManagedClientAmongCapacityUsers(t *testing.T) {
	clients := []any{
		map[string]any{"id": "other-1", "flow": "xtls-rprx-vision"},
		map[string]any{"id": "u", "flow": "xtls-rprx-vision"},
		map[string]any{"id": "other-2", "flow": "xtls-rprx-vision"},
	}
	in := Inbound{ID: 3, Remark: "dob:global-reality:01212", Port: 1212, Protocol: "vless", Enable: true,
		Settings: map[string]any{"decryption": "none", "encryption": "none", "clients": clients},
		StreamSettings: map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{
			"dest": "aws.amazon.com:443", "serverNames": []any{"aws.amazon.com"}, "privateKey": "secret", "shortIds": []any{"dad58810"},
		}},
	}
	if err := Validate(in, Expected{RemoteID: 3, Remark: "dob:global-reality:01212", Port: 1212, UUID: "u", Flow: "xtls-rprx-vision", Target: "aws.amazon.com:443", ServerNames: []string{"aws.amazon.com"}, PrivateKey: "secret", ShortID: "dad58810"}); err != nil {
		t.Fatal(err)
	}
}
func TestValidateRealityRejectsMissingManagedClientAmongCapacityUsers(t *testing.T) {
	in := Inbound{ID: 3, Remark: "k", Port: 1212, Protocol: "vless", Enable: true,
		Settings: map[string]any{"decryption": "none", "encryption": "none", "clients": []any{map[string]any{"id": "other", "flow": "xtls-rprx-vision"}}},
		StreamSettings: map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{
			"dest": "aws.amazon.com:443", "serverNames": []any{"aws.amazon.com"}, "privateKey": "secret", "shortIds": []any{"sid"},
		}},
	}
	if Validate(in, Expected{RemoteID: 3, Remark: "k", Port: 1212, UUID: "u", Flow: "xtls-rprx-vision", Target: "aws.amazon.com:443", ServerNames: []string{"aws.amazon.com"}, PrivateKey: "secret", ShortID: "sid"}) == nil {
		t.Fatal("expected managed client mismatch")
	}
}

func TestValidateServerNamesOrderIsSemantic(t *testing.T) {
	in := Inbound{
		ID: 1, Remark: "r", Port: 443, Protocol: "vless", Enable: true,
		Settings: map[string]any{"decryption": "none", "encryption": "none", "clients": []any{map[string]any{"id": "u", "flow": "xtls-rprx-vision"}}},
		StreamSettings: map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{
			"dest": "example.com:443", "serverNames": []any{"b.example", "a.example"}, "privateKey": "p", "shortIds": []any{"s"},
		}},
	}
	err := Validate(in, Expected{RemoteID: 1, Remark: "r", Port: 443, UUID: "u", Flow: "xtls-rprx-vision", Target: "example.com:443", ServerNames: []string{"a.example", "b.example"}, PrivateKey: "p", ShortID: "s"})
	if err != nil {
		t.Fatalf("semantic serverNames order should pass: %v", err)
	}
}
