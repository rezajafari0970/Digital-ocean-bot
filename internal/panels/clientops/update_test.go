package clientops

import (
	"encoding/json"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestPatchInboundClientPreservesStructureAndOtherClients(t *testing.T) {
	settings := `{"clients":[{"id":"u1","email":"old","enable":true,"totalGB":10,"expiryTime":20,"limitIp":1,"flow":"xtls-rprx-vision"},{"id":"u2","email":"keep","enable":true}]}`
	raw, _ := json.Marshal(map[string]any{
		"id": 1, "port": 443, "protocol": "vless",
		"settings":       settings,
		"streamSettings": map[string]any{"network": "tcp", "security": "reality"},
	})
	email := "new"
	quota := int64(99)
	payload, err := patchInboundClient(raw, "u1", ClientPatch{Email: &email, TotalGB: &quota})
	if err != nil {
		t.Fatal(err)
	}
	if payload["port"].(float64) != 443 || payload["protocol"].(string) != "vless" {
		t.Fatal("structural inbound fields changed")
	}
	stream := payload["streamSettings"].(map[string]any)
	if stream["security"] != "reality" {
		t.Fatal("stream settings changed")
	}

	settingsOut := payload["settings"].(map[string]any)
	clients := settingsOut["clients"].([]any)
	firstBytes, _ := json.Marshal(clients[0])
	secondBytes, _ := json.Marshal(clients[1])
	var first, second map[string]any
	_ = json.Unmarshal(firstBytes, &first)
	_ = json.Unmarshal(secondBytes, &second)
	if first["email"] != "new" || first["totalGB"].(float64) != 99 {
		t.Fatalf("target patch missing: %#v", first)
	}
	if second["id"] != "u2" || second["email"] != "keep" {
		t.Fatalf("non-target client changed: %#v", second)
	}
}

func TestPatchSatisfiedOnlyChecksRequestedFields(t *testing.T) {
	email := "new"
	c := mustClient(t, `{"id":"u1","email":"new","enable":true,"totalGB":10,"expiryTime":20,"limitIp":1,"flow":"x"}`)
	if !patchSatisfied(c, ClientPatch{Email: &email}) {
		t.Fatal("requested field already satisfied")
	}
	other := "other"
	if patchSatisfied(c, ClientPatch{Email: &other}) {
		t.Fatal("different requested field must require mutation")
	}
}

func mustClient(t *testing.T, raw string) sanaei.Client {
	t.Helper()
	var c sanaei.Client
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
