package clientops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestAllQueuedOriginsKeepRemoteFailureScoped(t *testing.T) {
	for _, variant := range []string{"lifecycle", "manual", "bulk", "lifecycle-timeout", "manual-timeout", "bulk-timeout"} {
		t.Run(variant, func(t *testing.T) {
			origin := strings.TrimSuffix(variant, "-timeout")
			db, j, rt, state := lifecycleFixture(t)
			mustIsolationSQL(t, db, `ALTER TABLE panel_instances ADD COLUMN base_url text;
 ALTER TABLE deployments ADD COLUMN postinstall_generation int DEFAULT 0;
 CREATE TABLE xui_panel_deployments(droplet_id uuid,generation int,username text,password_secret_ref text);
 INSERT INTO xui_panel_deployments SELECT droplet_id,0,'fixture','fixture' FROM panel_instances;`)
			var outage, corrupt atomic.Bool
			outage.Store(true)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/csrf-token" || r.URL.Path == "/login" {
					if outage.Load() {
						if strings.HasSuffix(variant, "-timeout") {
							<-r.Context().Done()
							return
						}
						w.WriteHeader(503)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": "fixture"})
					return
				}
				if corrupt.Load() {
					w.Write([]byte("{"))
					return
				}
				state.serve(w, r)
			}))
			defer srv.Close()
			mustIsolationSQL(t, db, "UPDATE panel_instances SET base_url=$1", srv.URL)
			switch origin {
			case "lifecycle":
				if !planLife(t, j, rt) {
					t.Fatal("plan")
				}
			case "manual":
				payload, _ := json.Marshal(map[string]any{"Client": sanaei.Client{ID: "manual-id", Email: "manual", Enable: true, Flow: "xtls-rprx-vision"}})
				if _, _, err := j.Reserve(context.Background(), Request{AccountID: rt.AccountID, PanelID: lifePanel, InboundID: 1, ClientID: "manual-id", Kind: KindCreate, IdempotencyKey: "manual-fixture", Payload: payload}); err != nil {
					t.Fatal(err)
				}
			case "bulk":
				mustIsolationSQL(t, db, `UPDATE bulk_client_execution_gate SET enabled=true,kill_switch=false,panel_id='33333333-3333-4333-8333-333333333333',inbound_id=1,max_batch_size=10,remaining_batches=10,expires_at=now()+interval '1 hour'`)
				if _, ok, err := j.ReserveBulk(context.Background(), rt.AccountID, lifePanel, 1, bulkTestPayload(3)); err != nil || !ok {
					t.Fatal(ok, err)
				}
			}

			factory := sanaei.RuntimeFactory{DB: db, Secrets: isolationSecrets{}, Timeout: 150 * time.Millisecond}
			var observed []string
			ex := Executor{Journal: j, Runtimes: &sanaei.RuntimeManager{Factory: factory}, Observe: func(s string) { observed = append(observed, s) }}
			if ok, err := ex.RunOne(context.Background()); err != nil || !ok {
				t.Fatal(ok, err)
			}
			var jobID, jobState string
			var attempts int
			if err := db.QueryRow("SELECT id::text,state,attempts FROM client_mutation_jobs").Scan(&jobID, &jobState, &attempts); err != nil || jobState != "PENDING" || attempts != 0 {
				t.Fatal(jobState, attempts, err)
			}
			if len(observed) != 2 || observed[1] != "ISOLATED" {
				t.Fatal(observed)
			}
			gate, err := j.Gate(context.Background())
			if err != nil || !gate.Enabled || gate.KillSwitch {
				t.Fatal(gate, err)
			}
			ex.Runtimes = &sanaei.RuntimeManager{Factory: factory}
			if ok, err := ex.RunOne(context.Background()); err != nil || ok {
				t.Fatal("restart ignored cooldown", ok, err)
			}
			outage.Store(false)
			corrupt.Store(true)
			for attempt := 1; attempt <= 3; attempt++ {
				mustIsolationSQL(t, db, "UPDATE client_mutation_panel_health SET retry_after=now()-interval '1 second' WHERE state='COOLDOWN'")
				mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET next_retry_at=now()-interval '1 second'")
				if ok, err := ex.RunOne(context.Background()); err != nil || !ok {
					t.Fatal("remote error escaped", ok, err)
				}
			}
			got, err := j.Get(context.Background(), jobID)
			if err != nil || got.State != StateFailed {
				t.Fatal(got, err)
			}
			var health string
			if err := db.QueryRow("SELECT state FROM client_mutation_panel_health").Scan(&health); err != nil || health != "QUARANTINED" {
				t.Fatal(health, err)
			}
			gate, err = j.Gate(context.Background())
			if err != nil || !gate.Enabled || gate.KillSwitch {
				t.Fatal(gate, err)
			}
			corrupt.Store(false)
			if ok, err := ex.RunOne(context.Background()); err != nil || ok {
				t.Fatal("quarantine auto-cleared", ok, err)
			}
			state.mu.Lock()
			posts := state.postCreates + state.postDeletes + state.postUpdates
			state.mu.Unlock()
			if posts != 0 {
				t.Fatal("fault triggered mutation", posts)
			}
		})
	}
}
func TestSingleClientInventoryCannotTurnMalformedDataIntoAbsence(t *testing.T) {
	for _, settings := range []any{map[string]any{}, nil, map[string]any{"clients": nil}, map[string]any{"clients": map[string]any{}}, map[string]any{"clients": []any{nil}}, map[string]any{"clients": []any{map[string]any{"email": "missing-id"}}}, "null"} {
		raw, _ := json.Marshal(map[string]any{"settings": settings})
		_, _, err := clientFromInbound(raw, "absent")
		if err == nil {
			t.Fatal("malformed inventory proved absence", settings)
		}
		if _, known := classifyPanelFailure(err, 1); !known {
			t.Fatal("inventory error escaped panel", err)
		}
	}
	raw := json.RawMessage(`{"settings":{"clients":[]}}`)
	if _, found, err := clientFromInbound(raw, "absent"); found || err != nil {
		t.Fatal(found, err)
	}
}

func TestDuplicateOrLaterMalformedInventoryCannotProveIdentity(t *testing.T) {
	for _, clients := range []any{
		[]any{map[string]any{"id": "target"}, map[string]any{"id": "target"}},
		[]any{map[string]any{"id": "other"}, map[string]any{"id": "other"}},
		[]any{map[string]any{"id": "target"}, nil},
	} {
		raw, _ := json.Marshal(map[string]any{"settings": map[string]any{"clients": clients}})
		if _, _, err := clientFromInbound(raw, "target"); err == nil {
			t.Fatal("ambiguous typed inventory accepted")
		}
		if _, _, err := clientMapFromInbound(raw, "target"); err == nil {
			t.Fatal("ambiguous raw inventory accepted")
		}
	}
}
func TestCommittedBulkPOSTTimeoutReconcilesWithoutDuplicate(t *testing.T) {
	db, j, rt, state := lifecycleFixture(t)
	mustIsolationSQL(t, db, `ALTER TABLE panel_instances ADD COLUMN base_url text;
 ALTER TABLE deployments ADD COLUMN postinstall_generation int DEFAULT 0;
 CREATE TABLE xui_panel_deployments(droplet_id uuid,generation int,username text,password_secret_ref text);
 INSERT INTO xui_panel_deployments SELECT droplet_id,0,'fixture','fixture' FROM panel_instances;`)
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/csrf-token" || r.URL.Path == "/login" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": "fixture"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "clients/bulkCreate") {
			posts.Add(1)
			state.serve(httptest.NewRecorder(), r)
			<-r.Context().Done()
			return
		}
		state.serve(w, r)
	}))
	defer srv.Close()
	mustIsolationSQL(t, db, "UPDATE panel_instances SET base_url=$1", srv.URL)
	if !planLife(t, j, rt) {
		t.Fatal("plan")
	}
	ex := Executor{Journal: j, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: isolationSecrets{}, Timeout: 150 * time.Millisecond}}}
	if ok, err := ex.RunOne(context.Background()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	var got string
	if err := db.QueryRow("SELECT state FROM client_mutation_jobs").Scan(&got); err != nil || got != "SUCCEEDED" {
		t.Fatal(got, err)
	}
	if ok, err := ex.RunOne(context.Background()); err != nil || ok {
		t.Fatal(ok, err)
	}
	if posts.Load() != 1 {
		t.Fatal("duplicate POST", posts.Load())
	}
}

type isolatedSecretHook func(context.Context, string, string) ([]byte, error)

func (f isolatedSecretHook) Get(ctx context.Context, a, b string) ([]byte, error) {
	return f(ctx, a, b)
}
func TestLifecycleScopeRevokedAfterClaimDoesNotCloseFleet(t *testing.T) {
	for _, scope := range []string{"panel_retiring", "operator_pause"} {
		t.Run(scope, func(t *testing.T) {
			db, j, rt, state := lifecycleFixture(t)
			mustIsolationSQL(t, db, `ALTER TABLE panel_instances ADD COLUMN base_url text;
 ALTER TABLE deployments ADD COLUMN postinstall_generation int DEFAULT 0;
 CREATE TABLE xui_panel_deployments(droplet_id uuid,generation int,username text,password_secret_ref text);
 INSERT INTO xui_panel_deployments SELECT droplet_id,0,'fixture','fixture' FROM panel_instances;`)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/csrf-token" || r.URL.Path == "/login" {
					json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": "fixture"})
					return
				}
				state.serve(w, r)
			}))
			defer srv.Close()
			mustIsolationSQL(t, db, "UPDATE panel_instances SET base_url=$1", srv.URL)
			if !planLife(t, j, rt) {
				t.Fatal("plan")
			}
			secrets := isolatedSecretHook(func(ctx context.Context, _, _ string) ([]byte, error) {
				q := "UPDATE droplets SET state='RETIRING'"
				if scope == "operator_pause" {
					q = "UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true"
				}
				_, err := db.ExecContext(ctx, q)
				return []byte("fixture"), err
			})
			ex := Executor{Journal: j, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: time.Second}}}
			if ok, err := ex.RunOne(context.Background()); err != nil || !ok {
				t.Fatal("expected gate refusal escaped globally", ok, err)
			}
			gate, err := j.Gate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if (scope == "panel_retiring") != gate.Enabled {
				t.Fatal("gate changed without operator", scope, gate)
			}
			var attempts, budget int
			if err := db.QueryRow("SELECT attempts FROM client_mutation_jobs").Scan(&attempts); err != nil || attempts != 1 {
				t.Fatal("claim attempt changed", attempts, err)
			}
			if err := db.QueryRow("SELECT remaining_operations FROM bulk_lifecycle_scopes").Scan(&budget); err != nil || budget != 29 {
				t.Fatal("authorization refunded/reset", budget, err)
			}
			var stateJob, last string
			if err := db.QueryRow("SELECT state,last_error FROM client_mutation_jobs").Scan(&stateJob, &last); err != nil || stateJob != "PENDING" || last != "EXECUTION_SCOPE_UNAVAILABLE" {
				t.Fatal(stateJob, last, err)
			}
			state.mu.Lock()
			posts := state.postCreates + state.postDeletes + state.postUpdates
			state.mu.Unlock()
			if posts != 0 {
				t.Fatal("revoked scope mutated panel", posts)
			}
			if scope == "panel_retiring" {
				if ok, err := ex.RunOne(context.Background()); err != nil || ok {
					t.Fatal(ok, err)
				}
				if err := db.QueryRow("SELECT state FROM client_mutation_jobs").Scan(&stateJob); err != nil || stateJob != "OBSOLETE" {
					t.Fatal(stateJob, err)
				}
			}
		})
	}
}

func TestBulkInventoryValidatesUnrelatedRecordsBeforeDeciding(t *testing.T) {
	wanted := []sanaei.Client{{ID: "target", Email: "target", Enable: true}}
	for _, records := range [][]sanaei.Client{
		{wanted[0], wanted[0]},
		{wanted[0], {ID: "other", Email: "other"}, {ID: "other", Email: "other"}},
		{wanted[0], {}},
		{wanted[0], {ID: "other", Email: "one"}, {ID: "another", Email: "one"}},
	} {
		raw, _ := json.Marshal(map[string]any{"id": 1, "enable": true, "settings": map[string]any{"clients": records}})
		for _, observe := range []func([]json.RawMessage, int64, []sanaei.Client) ([]sanaei.Client, []sanaei.Client, error){bulkObserved, bulkDeleteObserved} {
			if _, _, err := observe([]json.RawMessage{raw}, 1, wanted); err == nil {
				t.Fatal("ambiguous snapshot accepted", records)
			}
		}
	}
	// Same unrelated global client in two DIFFERENT inbounds is a valid native
	// attachment; owned target sharing remains rejected by the scope checks.
	raws := []json.RawMessage{}
	for _, id := range []int{1, 2} {
		raw, _ := json.Marshal(map[string]any{"id": id, "enable": true, "settings": map[string]any{"clients": []sanaei.Client{{ID: "shared", Email: "shared"}}}})
		raws = append(raws, raw)
	}
	if _, missing, err := bulkObserved(raws, 1, wanted); err != nil || len(missing) != 1 {
		t.Fatal(missing, err)
	}
}
