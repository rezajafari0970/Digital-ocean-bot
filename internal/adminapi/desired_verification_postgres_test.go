package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/desired"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/realitycontract"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type desiredFaultSecrets struct{}

func (desiredFaultSecrets) Get(_ context.Context, _ string, ref string) ([]byte, error) {
	switch ref {
	case "uuid":
		return []byte("b530c072-21c6-405d-95d8-fa78afa2d62e"), nil
	case "private":
		return []byte("private"), nil
	case "test":
		return []byte("test"), nil
	}
	return nil, errors.New("unexpected secret")
}
func (desiredFaultSecrets) Put(context.Context, string, string, string, []byte) error {
	return errors.New("unexpected secret write")
}

func TestDesiredVerificationFailureDoesNotInitiateNativeMutation(t *testing.T) {
	for _, fault := range []string{"metadata_sql", "missing_public_key", "missing_key_and_mismatch", "malformed_reality", "incomplete_reality", "malformed_clients", "matching", "structural_mismatch"} {
		t.Run(fault, func(t *testing.T) {
			db := adminTestDB(t)
			const managed = "dob:global-reality:00443"
			payload, err := (realitycontract.Managed{Remark: managed, Port: 443, UUID: "b530c072-21c6-405d-95d8-fa78afa2d62e", Email: "managed-443", Target: "www.example.com", TargetPort: 443, SNI: "www.example.com", ServerNames: []string{"www.example.com"}, PrivateKey: "private", ShortID: "aabb"}).Payload()
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(payload)
			var inbound map[string]any
			if err = json.Unmarshal(encoded, &inbound); err != nil {
				t.Fatal(err)
			}
			inbound["id"] = 1
			if fault == "structural_mismatch" || fault == "missing_key_and_mismatch" {
				inbound["streamSettings"].(map[string]any)["realitySettings"].(map[string]any)["dest"] = "changed.example:443"
			}
			if fault == "malformed_reality" {
				inbound["streamSettings"].(map[string]any)["realitySettings"].(map[string]any)["dest"] = 123
			}
			if fault == "incomplete_reality" {
				delete(inbound["streamSettings"].(map[string]any)["realitySettings"].(map[string]any), "serverNames")
			}
			if fault == "malformed_clients" {
				inbound["settings"].(map[string]any)["clients"] = []any{123}
			}
			var mu sync.Mutex
			mutations := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/csrf-token"):
					fmt.Fprint(w, `{"success":true,"obj":"csrf"}`)
				case strings.HasSuffix(r.URL.Path, "/login"):
					fmt.Fprint(w, `{"success":true}`)
				case strings.HasSuffix(r.URL.Path, "/inbounds/list"):
					json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": []any{inbound}})
				case strings.Contains(r.URL.Path, "/inbounds/get/"):
					json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": inbound})
				case r.Method == "POST":
					mutations++
					var next map[string]any
					if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
						t.Error(err)
					}
					for key, value := range next {
						inbound[key] = value
					}
					inbound["id"] = 1
					fmt.Fprint(w, `{"success":true}`)
				default:
					t.Errorf("unexpected endpoint %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer remote.Close()
			_, droplet, panel := seedPanel(t, db, remote.URL)
			sqlMust(t, db, `UPDATE deployments SET host='127.0.0.1',profile_snapshot='{"ssh_user":"root","ssh_key_secret_ref":"ssh"}' WHERE droplet_id=$1`, droplet)
			sqlMust(t, db, `INSERT INTO reality_target_selections(panel_id,target,server_name,port,score,selected_at,server_names) VALUES($1,'www.example.com','www.example.com',443,1,now(),'["www.example.com"]')`, panel)
			pub := "public"
			if fault == "missing_public_key" || fault == "missing_key_and_mismatch" {
				pub = ""
			}
			sqlMust(t, db, `INSERT INTO reality_credentials(panel_id,managed_key,uuid_secret_ref,private_key_secret_ref,public_key,short_id) VALUES($1,$2,'uuid','private',$3,'aabb')`, panel, managed, pub)
			if fault == "metadata_sql" {
				sqlMust(t, db, `CREATE FUNCTION deny_export_metadata() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected export metadata failure'; END $$`)
				sqlMust(t, db, `CREATE TRIGGER deny_export_metadata BEFORE INSERT OR UPDATE ON inbound_export_metadata FOR EACH ROW EXECUTE FUNCTION deny_export_metadata()`)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			runtimes := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: desiredFaultSecrets{}}}
			runtime, err := runtimes.Acquire(ctx, panel)
			if err != nil {
				t.Fatal(err)
			}
			err = sanaei.WithConfigLock(ctx, db, panel, func(owned context.Context) error {
				return (desired.Service{DB: db, Secrets: desiredFaultSecrets{}, ManagedKey: managed, Port: 443, MutationPanels: map[string]bool{panel: true}, Runtime: runtime}).ReconcilePanel(owned, readyworker.Panel{ID: panel}, false)
			})
			mu.Lock()
			got := mutations
			mu.Unlock()
			switch fault {
			case "metadata_sql", "missing_public_key", "missing_key_and_mismatch", "malformed_reality", "incomplete_reality", "malformed_clients":
				if err == nil {
					t.Error("verification failure hidden")
				}
				if got != 0 {
					t.Errorf("verification failure caused %d native mutation(s): %v", got, err)
				}
			case "matching":
				if err != nil || got != 0 {
					t.Errorf("matching configuration: writes=%d err=%v", got, err)
				}
			case "structural_mismatch":
				if err != nil || got != 1 {
					t.Errorf("confirmed mismatch must still converge: writes=%d err=%v", got, err)
				}
			}
		})
	}
}
