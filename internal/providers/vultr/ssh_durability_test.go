package vultr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

const durabilityPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4f"

func TestSSHImportUncertainVerificationNeverDeletes(t *testing.T) {
	for _, scenario := range []string{"missing-id", "mismatched-key", "missing-read"} {
		t.Run(scenario, func(t *testing.T) {
			var posts, deletes, gets atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					posts.Add(1)
					if scenario == "missing-id" {
						w.Write([]byte("{}"))
						return
					}
					w.Write([]byte(`{"ssh_key":{"id":"claimed-id","name":"owned"}}`))
				case http.MethodGet:
					gets.Add(1)
					if scenario == "missing-read" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					w.Write([]byte(`{"ssh_key":{"id":"claimed-id","name":"foreign","ssh_key":"invalid"}}`))
				case http.MethodDelete:
					deletes.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer srv.Close()
			client := NewClient(srv.Client(), "synthetic")
			client.base = srv.URL
			_, err := (&Driver{client: client}).CreateSSHKey(context.Background(), "owned", durabilityPublicKey)
			if err == nil || posts.Load() != 1 || deletes.Load() != 0 {
				t.Fatalf("error=%v posts=%d deletes=%d", err, posts.Load(), deletes.Load())
			}
			if scenario == "missing-id" && gets.Load() != 0 {
				t.Fatal("missing identity used in verification")
			}
			if scenario == "mismatched-key" && !providers.IsClass(err, providers.ErrorAmbiguousOutcome) {
				t.Fatal("unproven import not ambiguous")
			}
		})
	}
}
func TestSSHInventoryCarriesExactPublicKeyAndIsolatesCursor(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v2/ssh-keys" || r.URL.Query().Get("per_page") != "500" || r.URL.Query().Get("token") != "" {
			t.Error("inventory endpoint or page scope changed")
		}
		name := "first"
		next := "https://foreign.invalid/other?cursor=next&per_page=1&token=foreign"
		if calls == 2 {
			name = "owned"
			next = ""
			if r.URL.Query().Get("cursor") != "next" {
				t.Error("cursor lost")
			}
		}
		if calls > 2 {
			t.Error("unexpected page")
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []map[string]string{{"id": name, "name": name, "ssh_key": durabilityPublicKey}}, "meta": map[string]any{"links": map[string]string{"next": next}}})
	}))
	defer srv.Close()
	client := NewClient(srv.Client(), "synthetic")
	client.base = srv.URL + "/v2"
	keys, err := (&Driver{client: client}).ListSSHKeys(context.Background())
	if err != nil || len(keys) != 2 || keys[1].PublicKey != durabilityPublicKey || calls != 2 {
		t.Fatalf("keys=%d calls=%d err=%v", len(keys), calls, err)
	}
}
