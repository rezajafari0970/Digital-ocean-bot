package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	secretstore "github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type trojanTestSecrets struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (s *trojanTestSecrets) Get(_ context.Context, _, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" || key == "test" {
		return []byte("test"), nil
	}
	v, ok := s.values[key]
	if !ok {
		return nil, secretstore.ErrSecretNotFound
	}
	return append([]byte(nil), v...), nil
}
func (s *trojanTestSecrets) Put(_ context.Context, _, key, _ string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = append([]byte(nil), value...)
	return nil
}

type trojanTestSSH struct{ calls int }

func (s *trojanTestSSH) Run(context.Context, provisioning.Target, []byte, string) (string, error) {
	s.calls++
	return "free", nil
}
func TestTrojanCreateLostResponseAndRestart(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	var mu sync.Mutex
	posts := 0
	var created map[string]any
	vless := json.RawMessage(`{"id":1,"tag":"inbound-443","protocol":"vless","port":443,"enable":true,"settings":{"clients":[{"id":"existing-vless","email":"existing","enable":true}]},"streamSettings":{"network":"tcp","security":"reality"}}`)
	protected := trojantcp.ProtectedHash([]json.RawMessage{vless})
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/csrf-token":
			fmt.Fprint(w, `{"success":true,"obj":"test-csrf"}`)
		case "/login":
			fmt.Fprint(w, `{"success":true}`)
		case "/panel/api/inbounds/list":
			rows := []json.RawMessage{vless}
			if created != nil {
				b, _ := json.Marshal(created)
				rows = append(rows, b)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": rows})
		case "/panel/api/inbounds/add":
			posts++
			if posts > 1 {
				t.Error("creation repeated after lost response")
			}
			if e := json.NewDecoder(r.Body).Decode(&created); e != nil {
				t.Error(e)
			}
			created["id"] = 7
			created["tag"] = "inbound-8443"
			w.WriteHeader(503)
			fmt.Fprint(w, `{"success":false,"msg":"response lost after commit"}`)
		default:
			t.Errorf("unexpected native request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	_, droplet, panel := seedPanel(t, db, remote.URL)
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.1' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE trojan_tcp_policy SET enabled=true,fleet=true")
	sqlMust(t, db, "INSERT INTO trojan_tcp_listeners(panel_id,port,managed_key,credential_ref,policy_revision,certificate_not_after) VALUES($1,8443,'dob:trojan-tcp:08443','trojan-fixture',1,now()+interval '5 days')", panel)
	secrets := &trojanTestSecrets{values: map[string][]byte{}}
	ssh := &trojanTestSSH{}
	makeService := func() trojantcp.Service {
		return trojantcp.Service{DB: db, Secrets: secrets, SSH: ssh,
			Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: time.Second}},
			Probe: func(context.Context, string, int) (trojantcp.TLSProof, error) {
				return trojantcp.TLSProof{NotAfter: time.Now().Add(5 * 24 * time.Hour), SHA256: "test"}, nil
			}}
	}
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true")
	if e := makeService().Reconcile(ctx, panel); e == nil {
		t.Fatal("closed gate accepted create")
	}
	if posts != 0 {
		t.Fatal("closed gate mutated")
	}
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL")
	t.Run("queued executor leaves Reality readable and cancels safely", func(t *testing.T) {
		holder, e := db.Conn(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer holder.Close()
		if _, e = holder.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); e != nil {
			t.Fatal(e)
		}
		defer holder.ExecContext(context.Background(), "SELECT pg_advisory_unlock(628341902731)")
		waitQueued := func() {
			t.Helper()
			limit := time.Now().Add(3 * time.Second)
			for time.Now().Before(limit) {
				var waiting bool
				e := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND classid=(628341902731>>32)::oid AND objid=(628341902731 & 4294967295)::oid AND NOT granted)").Scan(&waiting)
				if e != nil {
					t.Fatal(e)
				}
				if waiting {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("creation did not join executor queue")
		}
		cancelled, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- makeService().Reconcile(cancelled, panel) }()
		waitQueued()
		readable, cancelRead := context.WithTimeout(ctx, 300*time.Millisecond)
		e = sanaei.WithConfigLock(readable, db, panel, func(context.Context) error { return nil })
		cancelRead()
		if e != nil {
			stop()
			t.Fatal("queued Trojan blocked Reality configuration reads", e)
		}
		mu.Lock()
		noMutation := posts == 0
		mu.Unlock()
		if !noMutation {
			stop()
			t.Fatal("mutation crossed executor fence")
		}
		stop()
		select {
		case e := <-done:
			if e == nil {
				t.Fatal("cancelled queue succeeded")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("queue cancellation unbounded")
		}
		// A new attempt must acquire the released executor and complete normally.
		go func() { done <- makeService().Reconcile(ctx, panel) }()
		waitQueued()
		if _, e = holder.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)"); e != nil {
			t.Fatal(e)
		}
		select {
		case e := <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("released executor did not make progress")
		}
	})
	if e := makeService().Reconcile(ctx, panel); e != nil {
		t.Fatal(e)
	}
	// A process restart must adopt verified credentials, without another POST.
	if e := makeService().Reconcile(ctx, panel); e != nil {
		t.Fatal(e)
	}
	if posts != 1 {
		t.Fatal("expected one mutation", posts)
	}
	var state string
	var remoteID int
	var owners int
	if e := db.QueryRow("SELECT state,remote_id FROM trojan_tcp_listeners WHERE panel_id=$1", panel).Scan(&state, &remoteID); e != nil || state != "VERIFIED" || remoteID != 7 {
		t.Fatal(state, remoteID, e)
	}
	if e := db.QueryRow("SELECT count(*) FROM trojan_tcp_clients WHERE panel_id=$1", panel).Scan(&owners); e != nil || owners != 2 {
		t.Fatal("ownership not durable", owners, e)
	}
	b, _ := json.Marshal(created)
	if trojantcp.ProtectedHash([]json.RawMessage{vless, b}) != protected {
		t.Fatal("VLESS changed")
	}
	// Fast verification remains eligible even while renewal/creation is deferred and expiry is near.
	sqlMust(t, db, "UPDATE droplets SET expires_at=now()+interval '5 minutes' WHERE id=$1", droplet)
	sqlMust(t, db, "UPDATE trojan_tcp_listeners SET verified_at=now()-interval '4 minutes',next_attempt_at=now()+interval '1 hour' WHERE panel_id=$1", panel)
	calls := ssh.calls
	if e := makeService().RunVerifyOne(ctx); e != nil {
		t.Fatal(e)
	}
	var fresh bool
	if e := db.QueryRow("SELECT verified_at>now()-interval '10 seconds' FROM trojan_tcp_listeners WHERE panel_id=$1", panel).Scan(&fresh); e != nil || !fresh {
		t.Fatal("fast verification starved", e)
	}
	if posts != 1 || ssh.calls != calls {
		t.Fatal("verification performed provisioning")
	}
}
