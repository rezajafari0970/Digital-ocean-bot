package digitalocean

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSSHInventoryCarriesExactPublicKey(t *testing.T) {
	const key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4f"
	client := fixtureClient(t, func(r *http.Request) (int, string) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/account/keys" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := json.Marshal(map[string]any{"ssh_keys": []map[string]any{{"id": 42, "name": "owned", "public_key": key, "fingerprint": "legacy-md5"}}})
		return 200, string(body)
	})
	driver, _ := NewDriver(client)
	keys, err := driver.ListSSHKeys(context.Background())
	if err != nil || len(keys) != 1 || keys[0].PublicKey != key || keys[0].Name != "owned" || keys[0].ID != "42" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
}
