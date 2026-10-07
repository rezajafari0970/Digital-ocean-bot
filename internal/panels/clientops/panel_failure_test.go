package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type isolationSecrets struct{}

func (isolationSecrets) Get(context.Context, string, string) ([]byte, error) {
	return []byte("fixture-password"), nil
}

const isolatedPanel = "55555555-5555-4555-8555-555555555555"
const isolatedGeneration = "66666666-6666-4666-8666-666666666666"

func mustIsolationSQL(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func TestPanelFailureClassificationKeepsInternalFailuresGlobal(t *testing.T) {
	for _, err := range []error{ErrInvalidRequest, ErrUnsupportedKind, errors.New("database failed"), sql.ErrConnDone, context.DeadlineExceeded} {
		if _, ok := classifyPanelFailure(err, 1); ok {
			t.Fatalf("internal error isolated: %v", err)
		}
	}
	internalHTTP := &url.Error{Op: "Get", URL: "http://internal-secret-store.test", Err: context.DeadlineExceeded}
	if _, known := classifyPanelFailure(fmt.Errorf("secrets: %w", internalHTTP), 1); known {
		t.Fatal("internal HTTP classified as panel")
	}
	if f, known := classifyPanelFailure(fmt.Errorf("%w: %w", sanaei.ErrSessionRequest, internalHTTP), 1); !known || f.quarantine || f.beforeMutation {
		t.Fatal("typed native panel transport lost provenance", f, known)
	}
	// A bare circuit sentinel after mutation is not proof of pre-mutation safety.
	if _, ok := classifyPanelFailure(sanaei.ErrRuntimeCircuitOpen, 1); ok {
		t.Fatal("phase proof missing")
	}
	f, ok := classifyPanelFailure(&runtimeUnavailable{cause: sanaei.ErrRuntimeCircuitOpen}, 100)
	if !ok || !f.beforeMutation || f.quarantine {
		t.Fatal(f, ok)
	}
	f, ok = classifyPanelFailure(fmt.Errorf("read: %w", ErrVerify), 3)
	if !ok || !f.quarantine || f.beforeMutation {
		t.Fatal(f, ok)
	}
	f, ok = classifyPanelFailure(ErrClientConflict, 1)
	if !ok || !f.quarantine {
		t.Fatal(f, ok)
	}
}

func TestPanelIsolationStaleInventoryAndRejectionRetryBeforeQuarantine(t *testing.T) {
	for _, failure := range []error{ErrInboundMissing, sanaei.ErrMutationRejected, sanaei.ErrInventoryRejected, ErrVerify, sanaei.ErrSessionRequest} {
		for attempt := 1; attempt <= 3; attempt++ {
			f, ok := classifyPanelFailure(failure, attempt)
			if !ok || f.beforeMutation || f.quarantine != (attempt == 3) {
				t.Fatal(failure, attempt, f, ok)
			}
		}
	}
}

func TestPanelIsolationCircuitDoesNotStopHealthyLifecycleAndSurvivesRestart(t *testing.T) {
	db, j, goodRT, goodState := lifecycleFixture(t)
	ctx := context.Background()
	mustIsolationSQL(t, db, `ALTER TABLE panel_instances ADD COLUMN base_url text;
 ALTER TABLE deployments ADD COLUMN postinstall_generation int DEFAULT 0;
 CREATE TABLE xui_panel_deployments(droplet_id uuid,generation int,username text,password_secret_ref text);
 INSERT INTO droplets(id,state) VALUES('77777777-7777-4777-8777-777777777777','READY');
 INSERT INTO deployments(droplet_id,state) VALUES('77777777-7777-4777-8777-777777777777','PANEL_COMPLETE');
 INSERT INTO panel_instances(id,account_id,droplet_id,enabled) VALUES('55555555-5555-4555-8555-555555555555','11111111-1111-4111-8111-111111111111','77777777-7777-4777-8777-777777777777',true);
 INSERT INTO panel_inbound_inventory(panel_id,remote_id,present,enabled) VALUES('55555555-5555-4555-8555-555555555555',1,true,true);
 INSERT INTO bulk_user_generations(id,panel_id,inbound_id,purpose,marker) VALUES('66666666-6666-4666-8666-666666666666','55555555-5555-4555-8555-555555555555',1,'POLICY','isolated');
 INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,target_users,quota_bytes,lifetime_seconds,device_limit,users_per_second,max_batch_size,remaining_operations,expires_at) SELECT '55555555-5555-4555-8555-555555555555',inbound_id,'66666666-6666-4666-8666-666666666666',enabled,use_global_policy,allow_create,target_users,quota_bytes,lifetime_seconds,device_limit,users_per_second,max_batch_size,remaining_operations,expires_at FROM bulk_lifecycle_scopes;
 INSERT INTO xui_panel_deployments SELECT droplet_id,0,'fixture','fixture' FROM panel_instances;`)
	badState := &lifeServer{clients: map[string]sanaei.Client{"manual": {ID: "manual-2", Email: "manual", Enable: true, Flow: "xtls-rprx-vision"}}, used: map[string]int64{}}
	var fail atomic.Bool
	var corrupt atomic.Int32
	var authCalls atomic.Int32
	server := func(state *lifeServer, bad bool) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/csrf-token" || r.URL.Path == "/login" {
				if bad {
					authCalls.Add(1)
				}
				if bad && fail.Load() {
					w.WriteHeader(503)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": "fixture-csrf"})
				return
			}
			if bad && corrupt.Load() > 0 && strings.HasSuffix(r.URL.Path, "inbounds/list") {
				if corrupt.Load() == 1 {
					w.Write([]byte("{"))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": []any{map[string]any{"id": 1, "enable": true, "settings": "{"}}})
				return
			}
			state.serve(w, r)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	goodServer, badServer := server(goodState, false), server(badState, true)
	mustIsolationSQL(t, db, "UPDATE panel_instances SET base_url=CASE WHEN id=$1 THEN $2 ELSE $3 END", lifePanel, goodServer.URL, badServer.URL)
	client, err := sanaei.NewAPIClient(badServer.URL, sanaei.Credentials{Username: "fixture", Password: "fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	badRT := &sanaei.PanelRuntime{PanelID: isolatedPanel, AccountID: goodRT.AccountID, Session: sanaei.NewPanelSession(client)}
	if !planLife(t, j, badRT) || !planLife(t, j, goodRT) {
		t.Fatal("plans missing")
	}
	mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET created_at=now()-interval '1 minute' WHERE panel_id=$1", isolatedPanel)
	fail.Store(true)
	factory := sanaei.RuntimeFactory{DB: db, Secrets: isolationSecrets{}, Timeout: time.Second}
	manager := &sanaei.RuntimeManager{Factory: factory}
	for i := 0; i < 3; i++ {
		if _, err := manager.Acquire(ctx, isolatedPanel); err == nil {
			t.Fatal("fault not active")
		}
	}
	if _, err := manager.Acquire(ctx, isolatedPanel); !errors.Is(err, sanaei.ErrRuntimeCircuitOpen) {
		t.Fatal(err)
	}
	e := Executor{Journal: j, Runtimes: manager}
	if n, err := e.Drain(ctx); err != nil || n != 2 {
		t.Fatalf("drain=%d err=%v", n, err)
	}
	var state string
	var attempts, deferrals, budget int
	var delay float64
	if err := db.QueryRow(`SELECT state,attempts,(result->>'pre_mutation_deferrals')::int,EXTRACT(EPOCH FROM(next_retry_at-now())) FROM client_mutation_jobs WHERE panel_id=$1`, isolatedPanel).Scan(&state, &attempts, &deferrals, &delay); err != nil {
		t.Fatal(err)
	}
	if state != "PENDING" || attempts != 0 || deferrals != 1 || delay < 20 || delay > 31 {
		t.Fatal(state, attempts, deferrals, delay)
	}
	if err := db.QueryRow("SELECT remaining_operations FROM bulk_lifecycle_scopes WHERE panel_id=$1", isolatedPanel).Scan(&budget); err != nil || budget != 29 {
		t.Fatal("budget was refunded", budget, err)
	}
	if err := db.QueryRow("SELECT state FROM client_mutation_jobs WHERE panel_id=$1", lifePanel).Scan(&state); err != nil || state != "SUCCEEDED" {
		t.Fatal(state, err)
	}
	gate, err := j.Gate(ctx)
	if err != nil || !gate.Enabled || gate.KillSwitch {
		t.Fatal(gate, err)
	}
	before := authCalls.Load()
	restarted := Executor{Journal: j, Runtimes: &sanaei.RuntimeManager{Factory: factory}}
	if n, err := restarted.Drain(ctx); err != nil || n != 0 || authCalls.Load() != before {
		t.Fatal("restart bypassed cooldown", n, err)
	}
	fail.Store(false)
	mustIsolationSQL(t, db, "UPDATE client_mutation_panel_health SET retry_after=now()-interval '1 second' WHERE panel_id=$1", isolatedPanel)
	mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET next_retry_at=now()-interval '1 second' WHERE panel_id=$1", isolatedPanel)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := restarted.RunOne(ctx); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow("SELECT state,attempts FROM client_mutation_jobs WHERE panel_id=$1", isolatedPanel).Scan(&state, &attempts); err != nil || state != "SUCCEEDED" || attempts != 1 {
		t.Fatal(state, attempts, err)
	}
	var health int
	if err := db.QueryRow("SELECT count(*) FROM client_mutation_panel_health").Scan(&health); err != nil || health != 0 {
		t.Fatal(health, err)
	}
	badState.mu.Lock()
	posts, clients := badState.postCreates, len(badState.clients)
	badState.mu.Unlock()
	if posts != 1 || clients != 4 {
		t.Fatal("duplicate/lost clients", posts, clients)
	}
	// A later malformed panel response must never escape as a global failure.
	badState.mu.Lock()
	for email, c := range badState.clients {
		if email != "manual" {
			c.Enable = false
			badState.clients[email] = c
		}
	}
	badState.mu.Unlock()
	if !planLife(t, j, badRT) {
		t.Fatal("cleanup plan missing")
	}
	for attempt := 1; attempt <= 3; attempt++ {
		corrupt.Store(int32(attempt))
		mustIsolationSQL(t, db, "UPDATE client_mutation_panel_health SET retry_after=now()-interval '1 second' WHERE state='COOLDOWN'")
		mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET next_retry_at=now()-interval '1 second' WHERE state='PENDING'")
		if worked, err := restarted.RunOne(ctx); err != nil || !worked {
			t.Fatal("malformed panel stopped fleet", worked, err)
		}
		g, err := j.Gate(ctx)
		if err != nil || !g.Enabled || g.KillSwitch {
			t.Fatal(g, err)
		}
	}
	var quarantined string
	if err := db.QueryRow("SELECT state FROM client_mutation_panel_health WHERE panel_id=$1", isolatedPanel).Scan(&quarantined); err != nil || quarantined != "QUARANTINED" {
		t.Fatal(quarantined, err)
	}
	badState.mu.Lock()
	deletes := badState.postDeletes
	badState.mu.Unlock()
	if deletes != 0 {
		t.Fatal("malformed inventory caused mutation", deletes)
	}
}

func TestPanelIsolationQuarantineIsAtomicAndNeverAutoCleared(t *testing.T) {
	db, j, rt, _ := lifecycleFixture(t)
	ctx := context.Background()
	planLife(t, j, rt)
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	// A write failure must roll back both the health row and the job transition.
	mustIsolationSQL(t, db, `CREATE FUNCTION reject_isolation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='FAILED' THEN RAISE EXCEPTION 'fault injected'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_isolation BEFORE UPDATE ON client_mutation_jobs FOR EACH ROW EXECUTE FUNCTION reject_isolation();`)
	f := panelFailure{code: "CLIENT_IDENTITY_CONFLICT", quarantine: true}
	if err = j.recordPanelFailure(ctx, job, f); err == nil {
		t.Fatal("expected journal failure")
	}
	var n int
	db.QueryRow("SELECT count(*) FROM client_mutation_panel_health").Scan(&n)
	if n != 0 {
		t.Fatal("half committed health")
	}
	got, err := j.Get(ctx, job.ID)
	if err != nil || got.State != StateRunning {
		t.Fatal(got, err)
	}
	mustIsolationSQL(t, db, "DROP TRIGGER reject_isolation ON client_mutation_jobs")
	if err = j.recordPanelFailure(ctx, job, f); err != nil {
		t.Fatal(err)
	}
	got, err = j.Get(ctx, job.ID)
	if err != nil || got.State != StateFailed {
		t.Fatal(got, err)
	}
	if _, ok, err = j.Claim(ctx); err != nil || ok {
		t.Fatal("quarantine admitted claim", ok, err)
	}
	// Even a previously in-flight success cannot clear quarantine.
	mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET state='RUNNING' WHERE id=$1", job.ID)
	if err = j.Succeed(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	blocked, err := j.PanelBlocked(ctx, job.PanelID)
	if err != nil || !blocked {
		t.Fatal(blocked, err)
	}
	// Deferral cannot silently rearm quarantine either.
	mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET state='RUNNING',attempts=1 WHERE id=$1", job.ID)
	job.Attempts = 1
	if err = j.recordPanelFailure(ctx, job, panelFailure{code: "RUNTIME_UNAVAILABLE", beforeMutation: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = j.Get(ctx, job.ID)
	if got.State != StateFailed {
		t.Fatal(got.State)
	}
	gate, _ := j.Gate(ctx)
	if !gate.Enabled || gate.KillSwitch {
		t.Fatal("quarantine widened to fleet")
	}
}

func TestPanelIsolationPreMutationDeferralsBoundedWithoutBudgetReset(t *testing.T) {
	db, j, rt, _ := lifecycleFixture(t)
	ctx := context.Background()
	planLife(t, j, rt)
	for i := 1; i <= 6; i++ {
		mustIsolationSQL(t, db, "UPDATE client_mutation_panel_health SET retry_after=now()-interval '1 second'")
		mustIsolationSQL(t, db, "UPDATE client_mutation_jobs SET next_retry_at=now()-interval '1 second' WHERE state='PENDING'")
		job, ok, err := j.Claim(ctx)
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
		if err = j.recordPanelFailure(ctx, job, panelFailure{code: "RUNTIME_UNAVAILABLE", beforeMutation: true}); err != nil {
			t.Fatal(err)
		}
		var d float64
		var attempts, deferrals, budget int
		err = db.QueryRow(`SELECT EXTRACT(EPOCH FROM(next_retry_at-now())),attempts,(result->>'pre_mutation_deferrals')::int FROM client_mutation_jobs WHERE id=$1`, job.ID).Scan(&d, &attempts, &deferrals)
		if err != nil || d < 29 || d > 120 || attempts != 0 || deferrals != i {
			t.Fatal(d, attempts, deferrals, err)
		}
		db.QueryRow("SELECT remaining_operations FROM bulk_lifecycle_scopes").Scan(&budget)
		if budget != 30-i {
			t.Fatal("budget changed", budget)
		}
	}
}

func TestPanelIsolationInternalFailureRetainsDurableGlobalClosure(t *testing.T) {
	db, j, _, _ := lifecycleFixture(t)
	ctx := context.Background()
	err := j.FailCloseGateWithFailure(ctx, &ExecutionFailure{PanelID: lifePanel, JobID: "test-job", Cause: errors.New("database journal unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	g, err := j.Gate(ctx)
	if err != nil || g.Enabled || !g.KillSwitch {
		t.Fatal(g, err)
	}
	var code, panel, job string
	if err = db.QueryRow("SELECT last_failure_code,last_failure_panel_id,last_failure_job_id FROM client_mutation_execution_gate").Scan(&code, &panel, &job); err != nil || code != "EXECUTOR_INTERNAL_FAILURE" || panel != lifePanel || job != "test-job" {
		t.Fatal(code, panel, job, err)
	}
}

func TestPanelIsolationUnrelatedSuccessCannotEraseNewerCooldown(t *testing.T) {
	db, j, rt, _ := lifecycleFixture(t)
	ctx := context.Background()
	planLife(t, j, rt)
	a, ok, err := j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	var bid string
	err = db.QueryRow(`INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,state,attempts,idempotency_key,payload)
 SELECT account_id,panel_id,inbound_id+1,client_id,kind,'RUNNING',1,'second-isolation-job',payload FROM client_mutation_jobs WHERE id=$1 RETURNING id::text`, a.ID).Scan(&bid)
	if err != nil {
		t.Fatal(err)
	}
	b, err := j.Get(ctx, bid)
	if err != nil {
		t.Fatal(err)
	}
	// Both lock orders must retain B's causal fence after A succeeds.
	done := make(chan error, 2)
	go func() { done <- j.Succeed(ctx, a.ID) }()
	go func() {
		done <- j.recordPanelFailure(ctx, b, panelFailure{code: "RUNTIME_UNAVAILABLE", beforeMutation: true})
	}()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var lastJob string
	if err := db.QueryRow("SELECT last_job_id::text FROM client_mutation_panel_health WHERE panel_id=$1", lifePanel).Scan(&lastJob); err != nil || lastJob != bid {
		t.Fatal(lastJob, err)
	}
}

func TestPanelIsolationLifecycleDiagnosticsExcludeRemoteSecrets(t *testing.T) {
	secret := "remote-secret-password@example.test"
	report := map[string]any{"phase": "VERIFY_REQUIRED", "response": map[string]any{"skipped": []string{secret}}, "post_error": secret, "verify_error": secret, "observed": 2}
	sanitizeLifecycleReport(report, 2, 1, fmt.Errorf("%w: %s", sanaei.ErrMutationRejected, secret), fmt.Errorf("%w: %s", ErrVerify, secret))
	raw, err := json.Marshal(report)
	if err != nil || strings.Contains(string(raw), secret) || report["post_error"] != "PANEL_REJECTED" || report["verify_error"] != "VERIFICATION_FAILED" {
		t.Fatal(string(raw), err)
	}
}

func TestPanelIsolationIncompleteCrossInboundInventoryCannotProveAbsence(t *testing.T) {
	wanted := []sanaei.Client{{ID: "owned-id", Email: "owned-email", Enable: true}}
	for _, settings := range []string{`{}`, `{"clients":null}`, `{"clients":{}}`, `{"clients":""}`} {
		bad := json.RawMessage(`{"id":2,"enable":true,"settings":` + settings + `}`)
		target := json.RawMessage(`{"id":1,"enable":true,"settings":{"clients":[]}}`)
		for _, raws := range [][]json.RawMessage{{bad}, {target, bad}} {
			_, _, err := bulkObserved(raws, 1, wanted)
			if err == nil {
				t.Fatal("malformed create snapshot became absence", settings)
			}
			if _, known := classifyPanelFailure(err, 1); !known {
				t.Fatal("malformed create snapshot stops fleet", err)
			}
			_, _, err = bulkDeleteObserved(raws, 1, wanted)
			if err == nil {
				t.Fatal("malformed non-target inbound allowed email deletion", settings)
			}
			if _, known := classifyPanelFailure(err, 1); !known {
				t.Fatal("malformed delete snapshot stops fleet", err)
			}
		}
	}
}
