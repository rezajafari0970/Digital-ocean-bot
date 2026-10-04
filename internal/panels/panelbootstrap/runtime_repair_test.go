package panelbootstrap

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type repairSecrets struct{}

func (repairSecrets) Get(context.Context, string, string) ([]byte, error)       { return []byte("test"), nil }
func (repairSecrets) Put(context.Context, string, string, string, []byte) error { return nil }

func TestRepairOutcomeSurvivesCancellationAndFreshRecovery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		healthy       bool
		attempts      int
		initial, want string
	}{
		{"lost_response_recovered", true, 7, "READY", "READY"},
		{"cancelled_failed_repair", false, 3, "READY", "RETIRING"},
		{"below_threshold", false, 2, "READY", "READY"},
		{"deleted_not_rearmed", false, 7, "DELETED", "DELETED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := repairTestDB(t)
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if !tc.healthy {
					http.Error(w, "unavailable", 503)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/csrf-token":
					w.Write([]byte(`{"success":true,"obj":"test"}`))
				case "/login":
					w.Write([]byte(`{"success":true}`))
				case "/panel/api/inbounds/list":
					w.Write([]byte(`{"success":true,"obj":[]}`))
				default:
					t.Errorf("unexpected verification route %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			account, _ := sanaei.UUIDv4()
			droplet, _ := sanaei.UUIDv4()
			panel, _ := sanaei.UUIDv4()
			sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')", account)
			sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES($1,$2,$4,$3)", droplet, account, tc.initial, droplet)
			sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'test','{}')", panel, account)
			sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,$3,'PANEL_COMPLETE','done')", account, panel, droplet)
			sqlMust(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui',$4,'test')", panel, account, droplet, server.URL)
			sqlMust(t, db, "INSERT INTO xui_panel_deployments(id,account_id,droplet_id,username,password_secret_ref,port,web_path,state) VALUES(gen_random_uuid(),$1,$2,'test','test',8080,'/','COMPLETED')", account, droplet)
			sqlMust(t, db, "INSERT INTO panel_runtime_repairs(panel_id,attempts) VALUES($1,$2)", panel, tc.attempts)
			s := Service{DB: db, Secrets: repairSecrets{}}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			for i := 0; i < 2; i++ {
				err := s.recordRuntimeRepair(ctx, panel, context.DeadlineExceeded)
				if tc.healthy && err != nil {
					t.Fatal(err)
				}
				if !tc.healthy && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("lost failure: %v", err)
				}
			}
			var state, lastError string
			var attempts, events int
			var success bool
			if err := db.QueryRow("SELECT state FROM droplets WHERE id=$1", droplet).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != tc.want {
				t.Fatalf("state=%s want %s", state, tc.want)
			}
			if err := db.QueryRow("SELECT attempts,last_error,last_success_at IS NOT NULL FROM panel_runtime_repairs WHERE panel_id=$1", panel).Scan(&attempts, &lastError, &success); err != nil {
				t.Fatal(err)
			}
			if tc.healthy {
				if attempts != 0 || lastError != "" || !success {
					t.Fatal("recovered result not persisted")
				}
			} else if lastError == "" || success {
				t.Fatal("failed result not persisted")
			}
			if err := db.QueryRow("SELECT count(*) FROM lifecycle_events WHERE resource_id=$1", droplet).Scan(&events); err != nil {
				t.Fatal(err)
			}
			wantEvents := 0
			if tc.want == "RETIRING" {
				wantEvents = 1
			}
			if events != wantEvents {
				t.Fatalf("duplicate/missing lifecycle event %d", events)
			}
			if requests.Load() < 2 {
				t.Fatal("fresh verification was skipped")
			}
		})
	}
}
