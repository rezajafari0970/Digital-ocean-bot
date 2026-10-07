package adminapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRestoreIsAtomicRevisionCheckedAndNeverRepeatsCleanup(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	account, _, panel := seedPanel(t, db, "http://panel.test")
	sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", account)
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	sqlMust(t, db, "UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true,max_active_scopes=1")
	var gen string
	if err := db.QueryRow("INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,1,'POLICY','restore-test') RETURNING id::text", panel).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,max_batch_size,remaining_operations,expires_at) VALUES($1,1,$2,false,true,true,10,7,now()+interval '1 hour')", panel, gen)
	store := cleanup.Store{DB: db}
	job, err := store.Start(ctx, []string{panel})
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE panel_cleanup_targets SET state='FAILED',last_error='unverified' WHERE job_id=$1", job)
	sqlMust(t, db, "UPDATE panel_cleanup_jobs SET state='PAUSED' WHERE id=$1", job)
	// Retrying initial planning must not restart deletions.
	if _, err = store.Start(ctx, []string{panel}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Status(ctx, job)
	if err != nil || st.Status != "paused" {
		t.Fatal(st, err)
	}
	var rev int64
	if err = db.QueryRow("SELECT revision FROM global_config_policies WHERE policy_key='reality'").Scan(&rev); err != nil {
		t.Fatal(err)
	}
	s := Server{DB: db}
	call := func(revision int64, id string, role auth.Role) int {
		req := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"expected_revision":%d,"cleanup_job_id":%q}`, revision, id)))
		req = req.WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Role: role}))
		out := httptest.NewRecorder()
		s.restoreCapacityAutomation(out, req)
		return out.Code
	}
	if code := call(rev+1, job, auth.Admin); code != 409 {
		t.Fatal("stale policy", code)
	}
	if code := call(rev, "", auth.Admin); code != 409 {
		t.Fatal("different cleanup", code)
	}
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET panel_id=$1,inbound_id=1", panel)
	if code := call(rev, job, auth.Admin); code != 409 {
		t.Fatal("scoped gate widened", code)
	}
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET panel_id=NULL,inbound_id=NULL")
	conn, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); e != nil {
		t.Fatal(e)
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)")
	done := make(chan int, 1)
	go func() { done <- call(rev, job, auth.Admin) }()
	select {
	case code := <-done:
		t.Fatal("restore raced a mutation", code)
	case <-time.After(100 * time.Millisecond):
	}
	if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)"); e != nil {
		t.Fatal(e)
	}
	select {
	case code := <-done:
		if code != 200 {
			t.Fatal("restore", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restore stuck")
	}

	var enabled, gate, kill bool
	var budget int
	if err = db.QueryRow("SELECT g.enabled,m.enabled,m.kill_switch FROM global_config_policies g CROSS JOIN client_mutation_execution_gate m").Scan(&enabled, &gate, &kill); err != nil || !enabled || !gate || kill {
		t.Fatal(enabled, gate, kill, err)
	}
	if err = db.QueryRow("SELECT remaining_operations FROM bulk_lifecycle_scopes WHERE panel_id=$1", panel).Scan(&budget); err != nil || budget != 7 {
		t.Fatal("budget changed", budget, err)
	}
	st, err = store.Status(ctx, job)
	if err != nil || st.Status != "cancelled" || st.Failed != 1 {
		t.Fatal("audit changed", st, err)
	}
	if code := call(rev, job, auth.Admin); code != 409 {
		t.Fatal("response-loss duplicate", code)
	}
	if err = store.Resume(ctx, job); !errors.Is(err, cleanup.ErrResumeState) {
		t.Fatal("stale resume restarted cleanup", err)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM panel_cleanup_jobs").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

type failingPanelSteps struct{ workflow.Steps }

func (failingPanelSteps) ConfigurePanel(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return d, fmt.Errorf("%w: Process exited with status 1", provisioning.ErrSSHCommand)
}

func TestPostInstallFailureRetiresAndCrashRecoveryPreservesReadyServers(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	c := app.Container{DB: db}
	account, drop, _ := seedPanel(t, db, "http://panel.test")
	var dep, profile string
	if err := db.QueryRow("SELECT id::text,profile_id::text FROM deployments WHERE droplet_id=$1", drop).Scan(&dep, &profile); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE droplets SET state='PROVISIONING',expires_at=NULL WHERE id=$1", drop)
	sqlMust(t, db, "UPDATE deployments SET state='CONFIGURING_PANEL',current_step='panel' WHERE id=$1", dep)
	engine, err := c.PostInstallWorkflow(ctx, app.DeploymentConfig{})
	if err != nil {
		t.Fatal(err)
	}
	engine.Steps = failingPanelSteps{engine.Steps}
	if _, err = engine.Run(ctx, workflow.Request{DeploymentID: dep, AccountID: account, ProfileID: profile}); err == nil {
		t.Fatal("expected failure")
	}
	var state string
	if err = db.QueryRow("SELECT state FROM droplets WHERE id=$1", drop).Scan(&state); err != nil || state != "RETIRING" {
		t.Fatal("missing finalizer", state, err)
	}
	// Simulate a crash after committing FAILED, before lifecycle finalization.
	sqlMust(t, db, "UPDATE droplets SET state='PROVISIONING' WHERE id=$1", drop)
	sqlMust(t, db, "UPDATE deployments SET updated_at=now()-interval '10 minutes' WHERE id=$1", dep)
	_, ready, _ := seedPanel(t, db, "http://ready.test")
	sqlMust(t, db, "UPDATE deployments SET state='FAILED',updated_at=now()-interval '10 minutes' WHERE droplet_id=$1", ready)
	_, rearmed, _ := seedPanel(t, db, "http://rearmed.test")
	sqlMust(t, db, "UPDATE droplets SET state='PROVISIONING' WHERE id=$1", rearmed)
	sqlMust(t, db, "UPDATE deployments SET state='IMPORTING_DATABASE',updated_at=now()-interval '10 minutes' WHERE droplet_id=$1", rearmed)
	n, err := c.ReconcileFailedProvisioning(ctx)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	n, err = c.ReconcileFailedProvisioning(ctx)
	if err != nil || n != 0 {
		t.Fatal("not idempotent", n, err)
	}
	for id, want := range map[string]string{drop: "RETIRING", ready: "READY", rearmed: "PROVISIONING"} {
		if err = db.QueryRow("SELECT state FROM droplets WHERE id=$1", id).Scan(&state); err != nil || state != want {
			t.Fatal(id, state, want, err)
		}
	}
}

func TestRoutingSummaryExcludesRetiringAccounts(t *testing.T) {
	db := adminTestDB(t)
	a, _, live := seedPanel(t, db, "http://live.test")
	retired, d, _ := seedPanel(t, db, "http://retiring.test")
	sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", a)
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", d)
	sqlMust(t, db, "UPDATE accounts SET enabled=false,deletion_requested_at=now() WHERE id=$1", retired)
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,state,revision,verified_at) SELECT $1,'APPLIED',revision,now() FROM residential_routing_control", live)
	s := Server{DB: db}
	w := httptest.NewRecorder()
	s.residentialRoutingStatus(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total_panels":1`) || !strings.Contains(w.Body.String(), `"retiring_panels":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestCapacityIsolationStatusAndResumePreserveQuarantine(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	account, _, panel := seedPanel(t, db, "http://panel.test")
	sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", account)
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	sqlMust(t, db, "UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true")
	var generation string
	if err := db.QueryRow("INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,1,'POLICY','quarantine-test') RETURNING id::text", panel).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,max_batch_size,remaining_operations,expires_at) VALUES($1,1,$2,false,true,true,10,7,now()+interval '1 hour')", panel, generation)
	sqlMust(t, db, "INSERT INTO client_mutation_panel_health(panel_id,state,failures,reason_code) VALUES($1,'QUARANTINED',3,'VERIFICATION_FAILED')", panel)
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true,last_failure_code='EXECUTOR_INTERNAL_FAILURE',last_failure_at=now()")
	s := Server{DB: db}
	request := httptest.NewRequest("GET", "/", nil)
	status, err := s.capacityAutomationStatus(request)
	if err != nil || status["running"] != false || status["quarantined_panels"] != 1 {
		t.Fatal(status, err)
	}
	denied := httptest.NewRecorder()
	s.resumeCapacityAutomation(denied, httptest.NewRequest("POST", "/", nil))
	if denied.Code != 403 {
		t.Fatal("unauthorized resume", denied.Code)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = resumeCapacityTx(ctx, tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	var budget int
	if err = db.QueryRow("SELECT enabled,remaining_operations FROM bulk_lifecycle_scopes WHERE panel_id=$1", panel).Scan(&enabled, &budget); err != nil || enabled || budget != 7 {
		t.Fatal(enabled, budget, err)
	}
	status, err = s.capacityAutomationStatus(request)
	if err != nil || status["running"] != true || status["quarantined_panels"] != 1 {
		t.Fatal(status, err)
	}
	recorder := httptest.NewRecorder()
	s.configCapacity(recorder, request)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "\"automation\"") || !strings.Contains(recorder.Body.String(), "QUARANTINED") {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
}
