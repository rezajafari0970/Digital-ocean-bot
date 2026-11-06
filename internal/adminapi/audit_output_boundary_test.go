package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuditOutputKeepsPluginOwnershipAndTypedClientsPostgres(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(fmt.Sprint(owned), func(t *testing.T) {
			db := adminTestDB(t)
			remark := "dob:global-reality:00443"
			if owned {
				remark = configdriver.ManagedPrefix + "00000000-0000-0000-0000-000000000099"
			}
			inbound := map[string]any{"id": 1, "enable": true, "protocol": "vless", "port": 443, "remark": remark,
				"settings": map[string]any{"decryption": "none", "clients": []any{
					map[string]any{"id": "b530c072-21c6-405d-95d8-fa78afa2d62e", "enable": true},
					map[string]any{"enable": true}, map[string]any{"id": 123, "enable": true},
				}},
				"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverNames": []string{"www.example.com"}, "shortIds": []string{"aabb"}}},
			}
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/csrf-token"):
					fmt.Fprint(w, `{"success":true,"obj":"csrf"}`)
				case strings.HasSuffix(r.URL.Path, "/login"):
					fmt.Fprint(w, `{"success":true}`)
				case strings.HasSuffix(r.URL.Path, "/inbounds/list"):
					json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": []any{inbound}})
				default:
					t.Errorf("unexpected native request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer remote.Close()
			_, droplet, panel := seedPanel(t, db, remote.URL)
			sqlMust(t, db, "UPDATE deployments SET host='192.0.2.10' WHERE droplet_id=$1", droplet)
			sqlMust(t, db, "INSERT INTO inbound_export_metadata(panel_id,remote_id,public_key) VALUES($1,1,'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA')", panel)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			runtimes := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: desiredFaultSecrets{}}}
			runtime, err := runtimes.Acquire(ctx, panel)
			if err != nil {
				t.Fatal(err)
			}
			server := &Server{DB: db}
			records, structures, ok := server.collectRuntimeOutput(ctx, readyworker.Panel{ID: panel}, runtime)
			if !ok {
				t.Fatal("fixture output read failed")
			}
			if owned {
				if len(records) != 0 || len(structures) != 0 {
					t.Fatal("legacy metadata bypassed plugin publication ownership")
				}
				return
			}
			if len(records) != 1 || len(structures) != 1 {
				t.Fatalf("malformed clients exported or valid client lost: records=%d structures=%d", len(records), len(structures))
			}
			if strings.Contains(records[0].URI, "nil") {
				t.Fatal("missing email was rendered as a literal nil value")
			}
		})
	}
}
