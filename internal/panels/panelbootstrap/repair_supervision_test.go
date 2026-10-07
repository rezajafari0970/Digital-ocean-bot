package panelbootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
)

// A normal 20s panel deadline can be followed by a separate 20s unknown-outcome
// readback. Advancing the supervision clock inside real HTTP proves that the
// canceled primary task cannot be mistaken for a stalled completion phase.
func TestRepairReadbackHasIndependentSupervisionAfterCallerDeadline(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		name := "failed_readback"
		if healthy {
			name = "lost_response_recovered"
		}
		t.Run(name, func(t *testing.T) {
			db := repairTestDB(t)
			var seconds atomic.Int64
			registry := supervision.New(func() time.Time { return time.Unix(1000+seconds.Load(), 0) })
			module, err := registry.Register(context.Background(), "global-reality", supervision.Policy{Loop: time.Minute, Work: 30 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			task, finish, err := supervision.Begin(module, "work", 0)
			if err != nil {
				t.Fatal(err)
			}
			defer finish()
			ctx, cancel := context.WithCancel(task)
			seconds.Store(20)
			cancel()
			checks := make(chan error, 1)
			var checked atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if checked.CompareAndSwap(false, true) {
					seconds.Store(32)
					checks <- registry.Check()
				}
				if !healthy {
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
					t.Errorf("unexpected readback path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			account, _ := sanaei.UUIDv4()
			droplet, _ := sanaei.UUIDv4()
			panel, _ := sanaei.UUIDv4()
			sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')", account)
			sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES($1,$2,$3,'READY')", droplet, account, droplet)
			sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'test','{}')", panel, account)
			sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,$3,'PANEL_COMPLETE','done')", account, panel, droplet)
			sqlMust(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui',$4,'test')", panel, account, droplet, server.URL)
			sqlMust(t, db, "INSERT INTO xui_panel_deployments(id,account_id,droplet_id,username,password_secret_ref,port,web_path,state) VALUES(gen_random_uuid(),$1,$2,'test','test',8080,'/','COMPLETED')", account, droplet)
			sqlMust(t, db, "INSERT INTO panel_runtime_repairs(panel_id,attempts) VALUES($1,3)", panel)
			err = (Service{DB: db, Secrets: repairSecrets{}}).recordRuntimeRepair(ctx, panel, context.DeadlineExceeded)
			if healthy && err != nil {
				t.Fatal(err)
			}
			if !healthy && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost original failure: %v", err)
			}
			select {
			case err = <-checks:
				if err != nil {
					t.Fatalf("valid bounded readback falsely stalled: %v", err)
				}
			default:
				t.Fatal("fresh readback missing")
			}
			if err = registry.Check(); err != nil {
				t.Fatal(err)
			}
			if !registry.Snapshot().Healthy || module.Err() != nil {
				t.Fatal("healthy module canceled")
			}
			var state string
			var attempts int
			var success bool
			if err = db.QueryRow("SELECT d.state,r.attempts,r.last_success_at IS NOT NULL FROM droplets d JOIN panel_instances p ON p.droplet_id=d.id JOIN panel_runtime_repairs r ON r.panel_id=p.id WHERE p.id=$1", panel).Scan(&state, &attempts, &success); err != nil {
				t.Fatal(err)
			}
			if healthy && (state != "READY" || attempts != 0 || !success) {
				t.Fatalf("successful unknown outcome lost: %s %d %v", state, attempts, success)
			}
			if !healthy && (state != "RETIRING" || attempts != 3 || success) {
				t.Fatalf("failed readback policy changed: %s %d %v", state, attempts, success)
			}
		})
	}
}

func TestRepairCompletionStillDetectsStalledPhaseAndSibling(t *testing.T) {
	for _, tc := range []struct {
		name, phase string
		native      time.Duration
		sibling     bool
	}{
		{"verify_stalled", "verify", 20 * time.Second, false},
		{"persistence_stalled", "database", 3 * time.Second, false},
		{"sibling_not_hidden", "verify", 20 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(1000, 0)
			r := supervision.New(func() time.Time { return now })
			module, e := r.Register(context.Background(), "global-reality", supervision.Policy{Loop: time.Minute, Work: 30 * time.Second})
			if e != nil {
				t.Fatal(e)
			}
			task, done, e := supervision.Begin(module, "work", 0)
			if e != nil {
				t.Fatal(e)
			}
			defer done()
			if tc.sibling {
				_, end, e := supervision.Begin(module, "work", 0)
				if e != nil {
					t.Fatal(e)
				}
				defer end()
			}
			now = now.Add(20 * time.Second)
			canceled, cancel := context.WithCancel(task)
			cancel()
			phase, end, e := repairCompletionPhase(canceled, tc.phase, tc.native)
			if e != nil {
				t.Fatal(e)
			}
			defer end()
			if phase.Err() != nil {
				t.Fatal("caller deadline leaked into completion")
			}
			deadline, ok := phase.Deadline()
			if !ok || time.Until(deadline) > tc.native {
				t.Fatal("native bound missing")
			}
			if tc.sibling {
				now = now.Add(12 * time.Second)
			} else {
				now = now.Add(tc.native + 6*time.Second)
			}
			if e = r.Check(); !errors.Is(e, supervision.ErrStalled) || module.Err() == nil || r.Snapshot().Healthy {
				t.Fatalf("stall hidden: %v", e)
			}
		})
	}
}

func TestRepairCompletionRetainsRealNativeTimeout(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, finish, e := repairCompletionPhase(parent, "verify", 10*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatal(ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("completion lost native cancellation bound")
	}
}
