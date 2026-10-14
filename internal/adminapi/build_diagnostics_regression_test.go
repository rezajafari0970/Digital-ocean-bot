package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvisionErrorsOnlyLatestFailedStepIsCurrent(t *testing.T) {
	db := adminTestDB(t)
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	var run string
	if err := db.QueryRow(`INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'WAITING_INSTALLER','ssh') RETURNING id::text`, a, d).Scan(&run); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, `INSERT INTO provision_step_attempts(run_id,step,last_error) VALUES($1,'ssh','current failure')`, run)
	sqlMust(t, db, `INSERT INTO provision_events(id,run_id,step,state,error_code,error_fingerprint,error_message,created_at) VALUES(gen_random_uuid(),$1,'ssh','FAILED','OLD','old','recovered probe',now()-interval '1 hour'),(gen_random_uuid(),$1,'ssh','FAILED','NEW','new','current failure',now())`, run)
	read := func() map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		(&Server{DB: db}).provisionErrors(w, httptest.NewRequest("GET", "/?summary=1", nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	value := read()
	if value["current_failed_runs"] != float64(1) {
		t.Fatal(value)
	}
	for _, raw := range value["records"].([]any) {
		x := raw.(map[string]any)
		want := float64(0)
		if x["fingerprint"] == "new" {
			want = 1
		}
		if x["open_runs"] != want {
			t.Fatal(x)
		}
	}
	// A fresh retry cannot inherit the previous attempt's fingerprint.
	sqlMust(t, db, `UPDATE provision_step_attempts SET last_started_at=now()+interval '1 second' WHERE run_id=$1`, run)
	for _, raw := range read()["records"].([]any) {
		if raw.(map[string]any)["open_runs"] != float64(0) {
			t.Fatal(raw)
		}
	}
	// Count current failed runs even if their event could not be recorded.
	if read()["current_failed_runs"] != float64(1) {
		t.Fatal("missing-event failure hidden")
	}
	sqlMust(t, db, `UPDATE provision_runs SET state='COMPLETED',current_step='done' WHERE id=$1`, run)
	value = read()
	if value["current_failed_runs"] != float64(0) {
		t.Fatal(value)
	}
	for _, raw := range value["records"].([]any) {
		x := raw.(map[string]any)
		if x["recovered_runs"] != float64(1) || x["open_runs"] != float64(0) {
			t.Fatal(x)
		}
	}
	sqlMust(t, db, `UPDATE provision_runs SET state='FAILED',current_step='ssh' WHERE id=$1`, run)
	sqlMust(t, db, `UPDATE droplets SET state='DELETED' WHERE id=$1`, d)
	value = read()
	if value["current_failed_runs"] != float64(0) {
		t.Fatal(value)
	}
	for _, raw := range value["records"].([]any) {
		x := raw.(map[string]any)
		if x["retired_runs"] != float64(1) || x["recovered_runs"] != float64(0) {
			t.Fatal(x)
		}
	}
}
func TestProvisionErrorSummaryIsGlobalAndEmptyArrayCompatible(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	w := httptest.NewRecorder()
	s.provisionErrors(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Code, w.Body.String())
	}
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	var run string
	if err := db.QueryRow(`INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'COMPLETED','done') RETURNING id::text`, a, d).Scan(&run); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, `INSERT INTO provision_events(id,run_id,step,state,error_code,error_fingerprint) SELECT gen_random_uuid(),$1,'ssh','FAILED','FIXTURE','fingerprint-'||n FROM generate_series(1,205)n`, run)
	w = httptest.NewRecorder()
	s.provisionErrors(w, httptest.NewRequest("GET", "/?summary=1", nil))
	var value struct {
		Records []any `json:"records"`
		Total   int   `json:"historical_families"`
		Current int   `json:"current_failed_runs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || value.Total != 205 || len(value.Records) != 200 || value.Current != 0 {
		t.Fatal(w.Code, value)
	}
}
func TestProvisionDetailJSONAndReadFailures(t *testing.T) {
	db := adminTestDB(t)
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	var run, deploy string
	if err := db.QueryRow(`INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step,last_error) VALUES(gen_random_uuid(),$1,$2,'COMPLETED','done','password=secret-detail-value') RETURNING id::text`, a, d).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id::text FROM deployments WHERE droplet_id=$1`, d).Scan(&deploy); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, `INSERT INTO provision_events(id,run_id,step,state,metadata,error_message) VALUES(gen_random_uuid(),$1,'panel','WAITING_INSTALLER','{"installer_selected":true}','token=secret-event-value')`, run)
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("id", deploy)
	w := httptest.NewRecorder()
	s := &Server{DB: db}
	s.provisionActivity(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-detail-value") || strings.Contains(w.Body.String(), "secret-event-value") {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Events []struct {
			Metadata map[string]any `json:"metadata"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || result.Events[0].Metadata["installer_selected"] != true {
		t.Fatal(result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w = httptest.NewRecorder()
	s.provisionActivity(w, req.WithContext(ctx))
	if w.Code != 500 {
		t.Fatal("read failure presented as missing data", w.Code, w.Body.String())
	}
	req = httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	w = httptest.NewRecorder()
	s.provisionActivity(w, req)
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestProvisionErrorsInstallerGenerationAndMissingConfiguration(t *testing.T) {
	db := adminTestDB(t)
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	var run, deploy, installer string
	if err := db.QueryRow(`SELECT id::text FROM deployments WHERE droplet_id=$1`, d).Scan(&deploy); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'WAITING_INSTALLER','panel') RETURNING id::text`, a, d).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO installers(name,version,manifest,sha256) VALUES('diagnostic',1,'{}',repeat('d',64)) RETURNING id::text`).Scan(&installer); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, `UPDATE deployments SET state='WAITING_INSTALLER',current_step='provision' WHERE id=$1`, deploy)
	sqlMust(t, db, `INSERT INTO installer_runs(deployment_id,provision_run_id,installer_id,manifest_snapshot,manifest_sha256,state,generation) VALUES($1,$2,$3,'{}',repeat('d',64),'INSTALLING',1)`, deploy, run, installer)
	sqlMust(t, db, `INSERT INTO provision_step_attempts(run_id,step,last_error) VALUES($1,'installer-g1-diagnostic-v1-install','failed install')`, run)
	sqlMust(t, db, `INSERT INTO provision_events(id,run_id,step,state,error_code,error_fingerprint,error_message) VALUES(gen_random_uuid(),$1,'installer-g1-diagnostic-v1-install','PHASE_FAILED','COMMAND_FAILED','installer-fingerprint','failed install')`, run)
	check := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		(&Server{DB: db}).provisionErrors(w, httptest.NewRequest("GET", "/?summary=1", nil))
		var out struct {
			Current int              `json:"current_failed_runs"`
			Records []map[string]any `json:"records"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || out.Current != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	check(1)
	// An unrelated deployment on the same resource cannot change attribution.
	sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,installer_generation,updated_at) SELECT gen_random_uuid(),account_id,profile_id,droplet_id,'PANEL_COMPLETE','done',2,now()+interval '1 hour' FROM deployments WHERE id=$1`, deploy)
	check(1)
	// A newly selected generation must not inherit the old generation's failure.
	sqlMust(t, db, `UPDATE deployments SET installer_generation=2 WHERE id=$1`, deploy)
	check(0)
	// Missing configuration remains a current issue even without a recorded event.
	sqlMust(t, db, `UPDATE provision_runs SET last_error='installer not configured' WHERE id=$1`, run)
	check(1)
	// Old workers emitted the same missing-config message for selected handoffs.
	sqlMust(t, db, `INSERT INTO deployment_installer_selections(deployment_id,generation,installer_name,installer_version) VALUES($1,2,'diagnostic',1)`, deploy)
	check(0)
	for _, state := range []string{"INSTALL_COMPLETE", "IMPORTING_DATABASE", "PANEL_COMPLETE"} {
		sqlMust(t, db, `UPDATE deployments SET state=$2 WHERE id=$1`, deploy, state)
		check(0)
	}
	sqlMust(t, db, `UPDATE provision_runs SET last_error=NULL WHERE id=$1`, run)
	check(0)
}

type diagnosticSecretStub struct{}

func (diagnosticSecretStub) Get(context.Context, string, string) ([]byte, error) {
	return []byte("fixture"), nil
}
func TestProvisionTerminalInterruptedFinalAttemptIsCountedWithoutEvent(t *testing.T) {
	db := adminTestDB(t)
	a, d, _ := seedPanel(t, db, "http://fixture.invalid")
	ctx := context.Background()
	store := provisioning.SQLStore{DB: db}
	run, _, err := store.Reserve(ctx, provisioning.Run{AccountID: a, DropletID: d, State: provisioning.Pending, CurrentStep: "prepare"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.BeginStep(ctx, run.ID, "prepare", 1); err != nil {
		t.Fatal(err)
	}
	// Crash after final admission: no FinishStep and no diagnostic event exist.
	run, err = (provisioning.Engine{Store: store, Secrets: diagnosticSecretStub{}}).Execute(ctx, provisioning.Target{AccountID: a, DropletID: d}, provisioning.Plan{Scripts: []provisioning.ScriptStep{{Name: "prepare", Execute: "unused", Precheck: "reconcile", MaxAttempts: 1}}})
	if !errors.Is(err, provisioning.ErrStepRetryLimit) || run.State != provisioning.Failed {
		t.Fatal(run, err)
	}
	w := httptest.NewRecorder()
	(&Server{DB: db}).provisionErrors(w, httptest.NewRequest("GET", "/?summary=1", nil))
	var out struct {
		Current int   `json:"current_failed_runs"`
		Records []any `json:"records"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || out.Current != 1 || len(out.Records) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
}
