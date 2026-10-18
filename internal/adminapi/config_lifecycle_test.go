package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func TestUnifiedProfileResizeDeleteAndRecovery(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	var mu sync.Mutex
	native := map[int64]map[string]any{}
	users := map[string]string{}
	nextID := int64(20)
	writes, deletes := 0, 0
	dropReads, dropAfterDelete, failProof := false, false, true
	verifiedTags := []string{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		envelope := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": v}) }
		switch {
		case r.URL.Path == "/csrf-token":
			envelope("csrf")
		case r.URL.Path == "/login":
			envelope(nil)
		case r.URL.Path == "/panel/api/inbounds/list":
			if dropReads {
				w.WriteHeader(503)
				return
			}
			out := []any{}
			for _, v := range native {
				out = append(out, v)
			}
			envelope(out)
		case r.URL.Path == "/panel/api/inbounds/add" || strings.HasPrefix(r.URL.Path, "/panel/api/inbounds/update/"):
			writes++
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid body")
				return
			}
			id := nextID
			if v, ok := body["id"].(float64); ok {
				id = int64(v)
			} else {
				nextID++
			}
			body["id"] = id
			body["tag"] = fmt.Sprintf("inbound-%d", id)
			native[id] = body
			var settings struct{ Clients []struct{ ID, Email string } }
			raw, _ := body["settings"].(string)
			if json.Unmarshal([]byte(raw), &settings) != nil {
				t.Error("invalid settings")
			}
			for _, c := range settings.Clients {
				users[c.Email] = c.ID
			}
			w.WriteHeader(503) // fresh observation, never status alone, confirms it
		case strings.HasPrefix(r.URL.Path, "/panel/api/inbounds/del/"):
			deletes++
			id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/panel/api/inbounds/del/"), 10, 64)
			delete(native, id)
			dropReads = dropAfterDelete
			w.WriteHeader(503)
		case r.URL.Path == "/panel/api/clients/list":
			out := []any{}
			for email, id := range users {
				out = append(out, map[string]any{"uuid": id, "email": email, "enable": true})
			}
			envelope(out)
		case r.URL.Path == "/panel/api/clients/bulkDel":
			var body struct{ Emails []string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, email := range body.Emails {
				delete(users, email)
			}
			w.WriteHeader(503)
		default:
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	_, droplet, panel := seedPanel(t, db, remote.URL)
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.10' WHERE droplet_id=$1", droplet)
	// Every server counted as eligible must be enrolled, even near retirement.
	sqlMust(t, db, "UPDATE droplets SET expires_at=now()+interval '5 minutes' WHERE id=$1", droplet)
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL")
	secrets := &trojanTestSecrets{values: map[string][]byte{}}
	svc := configplane.Service{DB: db, Secrets: secrets, SSH: &trojanTestSSH{}, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: time.Second}}, Verify: func(_ context.Context, _ provisioning.Target, _ []byte, payload map[string]any) error {
		verifiedTags = append(verifiedTags, payload["remark"].(string))
		if failProof {
			return errors.New("temporary traffic failure")
		}
		return nil
	}, VerifyAbsent: func(context.Context, provisioning.Target, []byte, int) error { return nil }}
	api := &Server{DB: db}
	p := configplane.Profile{Name: "VMess", Enabled: true, Fleet: true, Classes: []string{"DIRECT", "RESIDENTIAL"}, Count: 1, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 17443}}
	status, p := profileSave(t, api, p, "POST")
	if status != 200 {
		t.Fatal(status)
	}
	if e := svc.RunOne(ctx); e == nil {
		t.Fatal("eligible near-expiry server was not enrolled or failed proof reported success")
	}
	var mutation, state string
	var revision int64
	if e := db.QueryRow("SELECT mutation_state,state,applied_revision FROM config_driver_listeners WHERE profile_id=$1", p.ID).Scan(&mutation, &state, &revision); e != nil || mutation != "CONFIRMED" || revision != p.Revision || state != "BLOCKED" {
		t.Fatal("native confirmation was coupled to failed proof", mutation, state, revision, e)
	}
	failProof = false
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	if writes != 1 {
		t.Fatal("proof retry repeated native write")
	}
	original := map[string]string{}
	for k, v := range users {
		original[k] = v
	}
	p.Enabled = false
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal("pause request", status)
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("pause", e)
	}
	if len(users) != 0 || len(native) != 0 {
		t.Fatal("pause left native users")
	}
	p.Enabled = true
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal("resume request", status)
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("resume", e)
	}
	if !reflect.DeepEqual(users, original) {
		t.Fatal("resume replaced identities")
	}

	p.Count = 3
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal("resize rejected")
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	if len(users) != 6 {
		t.Fatal("growth count", len(users))
	}
	for email, id := range original {
		if users[email] != id {
			t.Fatal("existing identity replaced")
		}
	}
	p.Count = 1
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal(status)
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(users, original) {
		t.Fatal("shrink left global users or replaced retained identity")
	}
	p.Spec = configdriver.Spec{Protocol: "vless", Network: "tcp", Security: "reality", Port: 17444, SNI: "example.com", Flow: "xtls-rprx-vision"}
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal("protocol/port edit rejected")
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(users, original) {
		t.Fatal("protocol edit replaced users")
	}
	sibling := configplane.Profile{Name: "SS", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "shadowsocks", Network: "tcp", Security: "none", Port: 17445}}
	status, sibling = profileSave(t, api, sibling, "POST")
	if status != 200 {
		t.Fatal(status)
	}
	if e := svc.Reconcile(ctx, sibling.ID, panel); e != nil {
		t.Fatal(e)
	}
	// An overdue first proof/retry must not wait behind a later scheduled refresh.
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '2 minutes',attempted_at=now() WHERE profile_id=$1", p.ID)
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 minute',attempted_at=now()-interval '10 minutes' WHERE profile_id=$1", sibling.ID)
	if e := svc.RunVerifyOne(ctx); e != nil {
		t.Fatal("due proof", e)
	}
	if verifiedTags[len(verifiedTags)-1] != configdriver.ManagedPrefix+p.ID {
		t.Fatal("overdue proof starved behind scheduled refresh")
	}
	var siblingRemote int64
	if e := db.QueryRow("SELECT remote_id FROM config_driver_listeners WHERE profile_id=$1", sibling.ID).Scan(&siblingRemote); e != nil {
		t.Fatal(e)
	}
	siblingBefore, _ := json.Marshal(native[siblingRemote])
	body, _ := json.Marshal(map[string]any{"revision": p.Revision})
	req := httptest.NewRequest("DELETE", "/", bytes.NewReader(body))
	req.SetPathValue("id", p.ID)
	res := httptest.NewRecorder()
	api.deleteConfigProfile(res, req)
	if res.Code != 202 {
		t.Fatal(res.Code, res.Body.String())
	}
	reuse := sibling
	reuse.ID = ""
	reuse.Revision = 0
	reuse.Spec.Port = p.Spec.Port
	if code, _ := profileSave(t, api, reuse, "POST"); code != 409 {
		t.Fatal("port released before native removal")
	}
	dropAfterDelete = true
	if e := svc.Reconcile(ctx, p.ID, panel); e == nil {
		t.Fatal("uncertain delete falsely confirmed")
	}
	dropReads = false
	dropAfterDelete = false
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("delete recovery", e)
	}
	if deletes != 2 {
		t.Fatal("confirmed-absent inbound deletion repeated", deletes)
	}
	if e := svc.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	deleted, e := configplane.Read(ctx, db, p.ID)
	if e != nil || deleted.DeletedAt == nil {
		t.Fatal("deletion did not complete", e)
	}
	siblingAfter, _ := json.Marshal(native[siblingRemote])
	if !bytes.Equal(siblingBefore, siblingAfter) || len(users) != 1 || len(native) != 1 {
		t.Fatal("deletion changed sibling or left profile users")
	}
	if code, _ := profileSave(t, api, reuse, "POST"); code != 200 {
		t.Fatal("verified deleted port not reusable")
	}
}

func TestUnifiedProfileCreationIdempotency(t *testing.T) {
	db := adminTestDB(t)
	api := &Server{DB: db}
	p := configplane.Profile{Name: "Independent", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 2, RequestID: "11111111-1111-4111-a111-111111111111", Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none"}}
	code, first := profileSave(t, api, p, "POST")
	if code != 200 {
		t.Fatal(code)
	}
	code, again := profileSave(t, api, p, "POST")
	if code != 200 || again.ID != first.ID || again.Spec.Port != first.Spec.Port {
		t.Fatal("lost-response retry duplicated creation")
	}
	p.Count = 3
	if code, _ = profileSave(t, api, p, "POST"); code != 409 {
		t.Fatal("reused request identity changed desired state")
	}
}

func TestUnifiedAuthorityRetiresLegacyWithoutBlockingCompletedPanels(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	_, _, first := seedPanel(t, db, "http://127.0.0.1:19001")
	_, _, second := seedPanel(t, db, "http://127.0.0.1:19002")
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports) VALUES('reality',true,'[443]') ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	id, e := (cleanup.Store{DB: db}).StartUnified(ctx)
	if e != nil {
		t.Fatal(e)
	}
	again, e := (cleanup.Store{DB: db}).StartUnified(ctx)
	if e != nil || again != id {
		t.Fatal("transition restarted", e)
	}
	if _, e = db.Exec("UPDATE global_config_policies SET enabled=true WHERE policy_key='reality'"); e == nil {
		t.Fatal("legacy authority resurrected")
	}
	p := configplane.Profile{Name: "Reality", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 2, Spec: configdriver.Spec{Protocol: "vless", Network: "tcp", Security: "reality", SNI: "example.com", Port: 443}}
	status, p := profileSave(t, &Server{DB: db}, p, "POST")
	if status != 200 {
		t.Fatal("new desired profile blocked by unrelated cleanup", status)
	}
	sqlMust(t, db, "UPDATE panel_cleanup_targets SET state='SUCCEEDED' WHERE job_id=$1 AND panel_id=$2", id, first)
	for _, tc := range []struct {
		id   string
		want bool
	}{{first, true}, {second, false}} {
		var eligible bool
		e = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM config_driver_profiles cp CROSS JOIN panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id WHERE cp.id=$1 AND pi.id=$2 AND `+configplane.Eligible+`)`, p.ID, tc.id).Scan(&eligible)
		if e != nil || eligible != tc.want {
			t.Fatal("panel retirement isolation", eligible, tc.want, e)
		}
	}
}

func TestUnifiedStatusCountsActualPublishedOutput(t *testing.T) {
	db := adminTestDB(t)
	fixtures := seedAccountOutput(t, db)
	x := fixtures[0]
	api := &Server{DB: db}
	p := configplane.Profile{Name: "VMess", Enabled: true, Fleet: false, Panels: []string{x.Panel}, Classes: []string{"DIRECT"}, Count: 2, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 17443}}
	status, p := profileSave(t, api, p, "POST")
	if status != 200 {
		t.Fatal(status)
	}
	raw, _ := json.Marshal(p.Spec)
	sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,remote_id,credential_ref,applied_spec,applied_revision,state,verified_at) VALUES($1,$2,'fixture',20,'ref',$3,1,'VERIFIED',now())", p.ID, x.Panel, raw)
	read := func() map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		api.getConfigProfiles(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var d struct{ Profiles []map[string]any }
		if json.Unmarshal(w.Body.Bytes(), &d) != nil || len(d.Profiles) != 1 {
			t.Fatal(w.Body.String())
		}
		return d.Profiles[0]
	}
	item := read()
	if item["ready"] != float64(0) || item["published"] != float64(0) || item["target"] != float64(2) {
		t.Fatal("verified listener was incorrectly counted as published", item)
	}
	creds, e := configdriver.NewCredentials(p.ID, x.Panel, p.Classes, p.Count)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range creds.Clients {
		sqlMust(t, db, "INSERT INTO config_driver_clients(profile_id,panel_id,client_id,email,route_class) VALUES($1,$2,$3,$4,'DIRECT')", p.ID, x.Panel, c.ID, c.Email)
		sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) SELECT $1,$2,$3,'DIRECT','DIRECT',revision FROM residential_routing_control", x.Panel, c.ID, c.Email)
		uri, e := configdriver.Builtins().URI(p.Spec, "192.0.2.1", "fixture", creds, c)
		if e != nil {
			t.Fatal(e)
		}
		sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,client_id,uri,config_profile_id,config_profile_revision) VALUES($1,$2,$3,$4,1)", x.Panel, c.ID, uri, p.ID)
	}
	item = read()
	if item["ready"] != float64(1) || item["published"] != float64(2) || item["state"] != "ready" {
		t.Fatal("fresh output count", item)
	}
	sqlMust(t, db, "UPDATE output_config_snapshots SET last_seen_at=now()-interval '20 seconds' WHERE config_profile_id=$1", p.ID)
	item = read()
	if item["ready"] != float64(0) || item["published"] != float64(0) || item["verified"] != float64(1) {
		t.Fatal("stale output counted as ready", item)
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", x.Account)
	item = read()
	if item["eligible"] != float64(0) || item["target"] != float64(0) {
		t.Fatal("disabled account counted as eligible", item)
	}

}
