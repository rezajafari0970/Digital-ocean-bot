package adminapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildActivitySummaryGlobalTerminalAndSecretSafety(t *testing.T) {
	db := adminTestDB(t)
	a, d, profile := seedPanel(t, db, "http://fixture.invalid")
	// Unique profiles keep the worker's admission constraint intact.
	sqlMust(t, db, `WITH profiles AS(INSERT INTO deployment_profiles(id,account_id,name,config) SELECT gen_random_uuid(),$1,'bulk-'||i,'{}' FROM generate_series(1,105)i RETURNING id) INSERT INTO deployments(id,account_id,profile_id,state,current_step,updated_at) SELECT gen_random_uuid(),$1,id,'PROVISIONING','ssh',now()-interval '1 hour' FROM profiles`, a)
	for _, state := range []string{"READY", "FAILED", "PANEL_COMPLETE", "INSTALL_FAILED", "INSTALL_ROLLED_BACK", "UNKNOWN", "WAITING_INSTALLER"} {
		sqlMust(t, db, `WITH profile AS(INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,$2,'{}') RETURNING id) INSERT INTO deployments(id,account_id,profile_id,state,current_step) SELECT gen_random_uuid(),$1,id,$2,'fixture' FROM profile`, a, state)
	}
	sqlMust(t, db, `UPDATE droplets SET state='DELETED' WHERE id=$1`, d)
	sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$3,$2,'PROVISIONING','ssh')`, a, d, profile)
	var id string
	if e := db.QueryRow(`SELECT id::text FROM deployments WHERE state='FAILED' LIMIT 1`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, `INSERT INTO operations(id,account_id,kind,idempotency_key,state,error_code,error_message) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET',$2,'failed','invalid_request','raw secret-value')`, a, "deploy:"+id+":create:region:us-east")
	s := Server{DB: db}
	w := httptest.NewRecorder()
	s.buildActivity(w, httptest.NewRequest("GET", "/api/v1/build-activity?summary=1", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-value") {
		t.Fatal(w.Code, w.Body.String())
	}
	var body struct {
		Records []struct {
			State          string
			Active         bool
			OperationError string `json:"operation_error"`
		}
		Active  int `json:"active_builds"`
		Waiting int `json:"waiting_builds"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if body.Active != 106 || body.Waiting != 1 || len(body.Records) != 100 {
		t.Fatal(body.Active, body.Waiting, len(body.Records))
	}
	found := false
	for _, r := range body.Records {
		if r.State == "FAILED" {
			found = true
			if r.Active || r.OperationError != "operation: invalid_request" {
				t.Fatal(r)
			}
		}
		if r.Active && (r.State == "PANEL_COMPLETE" || r.State == "INSTALL_FAILED" || r.State == "INSTALL_ROLLED_BACK" || r.State == "UNKNOWN") {
			t.Fatal(r)
		}
	}
	if !found {
		t.Fatal("missing failure")
	}
	w = httptest.NewRecorder()
	s.buildActivity(w, httptest.NewRequest("GET", "/api/v1/build-activity", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "[") {
		t.Fatal("array compatibility", w.Body.String())
	}
	var dashboard []byte
	if e := db.QueryRow(dashboardCountsSQL).Scan(&dashboard); e != nil {
		t.Fatal(e)
	}
	var counters map[string]any
	json.Unmarshal(dashboard, &counters)
	if counters["active_deployments"] != float64(106) {
		t.Fatal(counters)
	}
}
func TestProvisionErrorHistoryDoesNotMultiplyOrHideRecovered(t *testing.T) {
	db := adminTestDB(t)
	a, d, profile := seedPanel(t, db, "http://fixture.invalid")
	sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$3,$2,'FAILED','ssh')`, a, d, profile)
	var run string
	if e := db.QueryRow(`INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'COMPLETED','done') RETURNING id::text`, a, d).Scan(&run); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, `INSERT INTO provision_events(id,run_id,step,state,error_code,error_class,error_fingerprint,error_message) SELECT gen_random_uuid(),$1,'ssh','STAGE_FAILED','SSH_TIMEOUT','transport','fixture-fingerprint','bounded error' FROM generate_series(1,2)`, run)
	sqlMust(t, db, `INSERT INTO provision_step_attempts(run_id,step,last_error) VALUES($1,'ssh','old error')`, run)
	read := func() []map[string]any {
		w := httptest.NewRecorder()
		(&Server{DB: db}).provisionErrors(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out []map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	out := read()
	if len(out) != 1 || out[0]["occurrences"] != float64(2) || out[0]["affected_deployments"] != float64(2) || out[0]["open_runs"] != float64(0) {
		t.Fatal(out)
	}
	sqlMust(t, db, `UPDATE provision_runs SET state='WAITING_INSTALLER' WHERE id=$1`, run)
	out = read()
	if out[0]["open_runs"] != float64(1) || out[0]["waiting_runs"] != float64(1) {
		t.Fatal(out)
	}
	sqlMust(t, db, `UPDATE droplets SET state='DELETED' WHERE id=$1`, d)
	out = read()
	if out[0]["open_runs"] != float64(0) || out[0]["waiting_runs"] != float64(0) || out[0]["occurrences"] != float64(2) {
		t.Fatal(out)
	}
}
