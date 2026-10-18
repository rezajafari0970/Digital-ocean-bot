package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
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
	globalDetails := map[string]map[string]any{}
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
		case r.URL.Path == "/panel/api/server/scanRealityTargets":
			envelope([]any{map[string]any{"host": "example.com", "port": 443, "feasible": true, "certValid": true, "certChainValid": true, "tlsVersion": "1.3", "alpn": "h2", "latencyMs": 10}})
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
			var full map[string]any
			_ = json.Unmarshal([]byte(raw), &full)
			for _, item := range full["clients"].([]any) {
				c := item.(map[string]any)
				email := c["email"].(string)
				hw := any(float64(0))
				if previous := globalDetails[email]; previous != nil {
					hw = previous["limitHwid"]
				}
				g := map[string]any{}
				for k, v := range c {
					g[k] = v
				}
				g["uuid"] = c["id"]
				g["limitHwid"] = hw
				globalDetails[email] = g
				delete(c, "limitHwid")
			}
			rewritten, _ := json.Marshal(full)
			body["settings"] = string(rewritten)

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
				if g := globalDetails[email]; g != nil {
					out = append(out, g)
				} else {
					out = append(out, map[string]any{"uuid": id, "email": email, "enable": true})
				}
			}
			envelope(out)
		case strings.HasPrefix(r.URL.Path, "/panel/api/clients/get/"):
			envelope(map[string]any{"client": globalDetails[strings.TrimPrefix(r.URL.Path, "/panel/api/clients/get/")]})
		case strings.HasPrefix(r.URL.Path, "/panel/api/clients/update/"):
			email := strings.TrimPrefix(r.URL.Path, "/panel/api/clients/update/")
			var patch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&patch)
			g := globalDetails[email]
			if g == nil {
				w.WriteHeader(404)
				return
			}
			for k, v := range patch {
				g[k] = v
			}
			w.WriteHeader(503) // successful SET with lost acknowledgement
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
	// A legacy writer on another panel must not serialize unified create/delete.
	sqlMust(t, db, "UPDATE config_control_authority SET unified=true WHERE singleton")
	legacyWriter, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = legacyWriter.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); e != nil {
		t.Fatal(e)
	}
	defer func() {
		_, _ = legacyWriter.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()")
		legacyWriter.Close()
	}()

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
	// Independent limits reach native settings, and a failed Residential proof
	// cannot erase the authenticated Direct proof.
	p.ClassSettings = map[string]configdriver.ClassSettings{
		"DIRECT":      {Count: 1, LifetimeSeconds: 300, DeviceLimit: 2, Publish: true},
		"RESIDENTIAL": {Count: 1, LifetimeSeconds: 7200, DeviceLimit: 7, Publish: false},
	}
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal("independent settings rejected")
	}
	priorVerifier := svc.Verify
	svc.Verify = func(_ context.Context, _ provisioning.Target, _ []byte, payload map[string]any) error {
		payload["verified_classes"] = []string{"DIRECT"}
		return nil
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	var directProof, residentialProof bool
	if e := db.QueryRow("SELECT bool_or(route_class='DIRECT' AND verified_at IS NOT NULL),bool_or(route_class='RESIDENTIAL' AND verified_at IS NOT NULL) FROM config_driver_class_proofs WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&directProof, &residentialProof); e != nil || !directProof || residentialProof {
		t.Fatal("class proof coupled", e)
	}
	var proofRef string
	if e := db.QueryRow("SELECT credential_ref FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&proofRef); e != nil {
		t.Fatal(e)
	}
	var saved configdriver.Credentials
	if json.Unmarshal(secrets.values[proofRef], &saved) != nil {
		t.Fatal("saved credentials decode")
	}
	if len(saved.Clients) != 2 {
		t.Fatal("class credentials absent")
	}
	for _, c := range saved.Clients {
		want := p.ClassSettings[c.Class]
		if globalDetails[c.Email]["limitHwid"] != float64(want.DeviceLimit) {
			t.Fatal("native global device limit was not observed")
		}
		if c.ExpiresAt-c.CreatedAt != int64(want.LifetimeSeconds)*1000 || c.DeviceLimit != want.DeviceLimit || c.Publish != want.Publish {
			t.Fatal("class limit mismatch")
		}
	}
	for _, n := range native {
		raw, _ := n["settings"].(string)
		var settings struct{ Clients []map[string]any }
		if json.Unmarshal([]byte(raw), &settings) != nil {
			t.Fatal("native settings")
		}
		for _, c := range settings.Clients {
			if c["expiryTime"] == float64(0) {
				t.Fatal("native limits missing")
			}
		}
	}
	svc.Verify = priorVerifier
	p.ClassSettings = map[string]configdriver.ClassSettings{"DIRECT": {Count: 1, Publish: true}, "RESIDENTIAL": {Count: 1, Publish: true}}
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal(status)
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	// The production writer finishes native policy independently and releases the
	// panel for route reconciliation; a later proof worker completes publication.
	production := svc
	production.Verify = nil
	if e := production.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("deferred production proof", e)
	}
	var queuedStage string
	if e := db.QueryRow("SELECT mutation_state,state,stage FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&mutation, &state, &queuedStage); e != nil || mutation != "CONFIRMED" || state != "APPLYING" || queuedStage != "routes_pending" {
		t.Fatal("native writer waited for runtime", mutation, state, queuedStage, e)
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("deferred proof recovery", e)
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
	p.Spec = configdriver.Spec{Protocol: "vless", Network: "tcp", Security: "reality", Port: 17444, SNI: "example.com", SNIMode: "manual", Flow: "xtls-rprx-vision"}
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
	// Repeated lock contention must not postpone an older proof behind a
	// healthy protocol simply because its claim timestamp was refreshed.
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now()-interval '10 minutes',next_attempt_at=now()-interval '1 second' WHERE profile_id=$1", p.ID)
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now(),next_attempt_at=now()-interval '10 minutes' WHERE profile_id=$1", sibling.ID)
	if e := svc.RunVerifyOne(ctx); e != nil || verifiedTags[len(verifiedTags)-1] != configdriver.ManagedPrefix+p.ID {
		t.Fatal("older Reality proof lost its publication deadline", e)
	}
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now()-interval '10 minutes',next_attempt_at=now()-interval '1 second' WHERE profile_id=$1", sibling.ID)
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now(),next_attempt_at=now()-interval '10 minutes' WHERE profile_id=$1", p.ID)
	if e := svc.RunVerifyOne(ctx); e != nil || verifiedTags[len(verifiedTags)-1] != configdriver.ManagedPrefix+sibling.ID {
		t.Fatal("older Shadowsocks proof lost its publication deadline", e)
	}
	// Recovery consumers repair a newly failed class ahead of routine work.
	// Ordinary consumers still protect older positive proofs from starvation.
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now()-interval '2 minutes',next_attempt_at=now()-interval '30 seconds',error_code='' WHERE profile_id=$1", p.ID)
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now(),next_attempt_at=now()-interval '1 second',error_code='profile_class_traffic_retrying' WHERE profile_id=$1", sibling.ID)
	if e := svc.RunVerifyRecoveryOne(ctx); e != nil || verifiedTags[len(verifiedTags)-1] != configdriver.ManagedPrefix+sibling.ID {
		t.Fatal("recovery waited behind a healthy routine proof", e)
	}
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 second',error_code='profile_class_traffic_retrying' WHERE profile_id=$1", sibling.ID)
	if e := svc.RunVerifyOne(ctx); e != nil || verifiedTags[len(verifiedTags)-1] != configdriver.ManagedPrefix+p.ID {
		t.Fatal("recovery consumer assignment starved oldest routine proof", e)
	}
	sqlMust(t, db, "UPDATE config_driver_listeners SET error_code='',next_attempt_at=now()+interval '1 hour' WHERE panel_id=$1", panel)
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 second' WHERE profile_id=$1", p.ID)
	if e := svc.RunVerifyRecoveryOne(ctx); e != nil || verifiedTags[len(verifiedTags)-1] != configdriver.ManagedPrefix+p.ID {
		t.Fatal("idle recovery consumer did not help routine work", e)
	}
	busyService := svc
	busyService.Verify = nil
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()+interval '1 hour' WHERE panel_id=$1", panel)
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 second' WHERE profile_id=$1", p.ID)
	if e := sanaei.WithConfigLock(ctx, db, panel, func(c context.Context) error {
		err := busyService.RunVerifyOne(c)
		if err == nil || err.Error() != "profile_panel_busy" {
			t.Fatal("expected isolated panel contention", err)
		}
		var prompt, retained, unleased bool
		if err := db.QueryRow("SELECT next_attempt_at<now()+interval '3 seconds',state='VERIFIED' AND verified_at IS NOT NULL AND error_code='',lease_until IS NULL FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&prompt, &retained, &unleased); err != nil || !prompt || !retained || !unleased {
			t.Fatal("busy proof did not requeue promptly with prior evidence", prompt, retained, unleased, err)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	// Exercise the production batch SQL, independent class publication and
	// a sibling runtime failure, with actual public TCP sockets.
	tcpA, e := net.Listen("tcp", "127.0.0.1:17444")
	if e != nil {
		t.Fatal(e)
	}
	defer tcpA.Close()
	tcpB, e := net.Listen("tcp", "127.0.0.1:17445")
	if e != nil {
		t.Fatal(e)
	}
	defer tcpB.Close()
	sqlMust(t, db, "UPDATE deployments SET host='127.0.0.1' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 minute' WHERE panel_id=$1", panel)
	batch := &profileBatchFixtureSSH{failed: sibling.ID}
	batchService := svc
	batchService.SSH = batch
	if e := batchService.VerifyPanel(ctx, panel, p.ID); e != nil {
		t.Fatal("production batch", e)
	}
	if batch.jobs != 2 {
		t.Fatal("batch lost sibling", batch.jobs)
	}
	var goodState, badState string
	if e := db.QueryRow("SELECT state FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&goodState); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow("SELECT state FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", sibling.ID, panel).Scan(&badState); e != nil {
		t.Fatal(e)
	}
	if goodState != "VERIFIED" || badState != "BLOCKED" {
		t.Fatal("batch failures coupled", goodState, badState)
	}
	if e := db.QueryRow("SELECT bool_or(route_class='DIRECT' AND verified_at IS NOT NULL),bool_or(route_class='RESIDENTIAL' AND verified_at IS NOT NULL) FROM config_driver_class_proofs WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&directProof, &residentialProof); e != nil || !directProof || residentialProof {
		t.Fatal("batch class proof coupled", e)
	}
	// Cancelling a worker during deployment must not revoke fresh completed
	// proof for healthy profiles merely because its SSH read was interrupted.
	proofContext, cancelProof := context.WithCancel(ctx)
	batch.cancel = cancelProof
	if e := batchService.VerifyPanel(proofContext, panel, p.ID); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled proof", e)
	}
	batch.cancel = nil
	if e := db.QueryRow("SELECT state FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&goodState); e != nil || goodState != "VERIFIED" {
		t.Fatal("cancel erased previous positive proof", goodState, e)
	}
	// A failed sibling's early retry must not re-probe a healthy, not-due profile.
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()+interval '1 hour' WHERE profile_id=$1", p.ID)
	batch.jobs = 0
	if e := batchService.VerifyPanel(ctx, panel, sibling.ID); e != nil {
		t.Fatal(e)
	}
	if batch.jobs != 1 {
		t.Fatal("not-due healthy profile re-probed", batch.jobs)
	}
	// A known older core schedules exactly one shared-runtime replacement,
	// without revoking successful profile proof or changing client identities.
	batch.oldCore = true
	sqlMust(t, db, "UPDATE config_driver_listeners SET next_attempt_at=now()-interval '1 minute' WHERE panel_id=$1", panel)
	if e := batchService.VerifyPanel(ctx, panel, p.ID); e != nil {
		t.Fatal(e)
	}
	var pending int
	if e := db.QueryRow("SELECT count(*) FROM config_driver_listeners WHERE panel_id=$1 AND needs_reconcile AND next_attempt_at<=now()", panel).Scan(&pending); e != nil || pending != 1 {
		t.Fatal("shared runtime upgrade not isolated", pending, e)
	}
	if e := db.QueryRow("SELECT state FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&goodState); e != nil || goodState != "VERIFIED" {
		t.Fatal("runtime upgrade revoked positive proof", goodState, e)
	}
	batch.oldCore = false
	sqlMust(t, db, "UPDATE config_driver_listeners SET needs_reconcile=false WHERE panel_id=$1", panel)
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.10' WHERE droplet_id=$1", droplet)
	// Sanaei v3 stores HWID limits in the global client, omitting them from
	// inbound inventory. Deletion must use the same owned-policy observation
	// contract as creation, including expiring clients and nonzero device caps.
	p.ClassSettings = map[string]configdriver.ClassSettings{"DIRECT": {Count: 1, Publish: true, DeviceLimit: 5, LifetimeSeconds: 600}, "RESIDENTIAL": {Count: 1, Publish: true, DeviceLimit: 7, LifetimeSeconds: 3600}}
	status, p = profileSave(t, api, p, "PUT")
	if status != 200 {
		t.Fatal("managed limits before deletion", status)
	}
	if e := svc.Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("managed limits before deletion", e)
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
	detailRequest := httptest.NewRequest("GET", "/", nil)
	detailRequest.SetPathValue("id", p.ID)
	detailResponse := httptest.NewRecorder()
	api.configProfileServers(detailResponse, detailRequest)
	var detail struct{ Servers []struct{ Removing bool } }
	if e := json.Unmarshal(detailResponse.Body.Bytes(), &detail); e != nil || detailResponse.Code != 200 || len(detail.Servers) != 1 || !detail.Servers[0].Removing {
		t.Fatal("server details lost desired deletion state", detailResponse.Code, detailResponse.Body.String(), e)
	}

	reuse := sibling
	reuse.ID = ""
	reuse.Revision = 0
	reuse.Spec.Port = p.Spec.Port
	if code, _ := profileSave(t, api, reuse, "POST"); code != 409 {
		t.Fatal("port released before native removal")
	}
	if e := sanaei.WithConfigLock(ctx, db, panel, func(c context.Context) error {
		err := svc.Reconcile(c, p.ID, panel)
		if err == nil || err.Error() != "profile_panel_busy" {
			t.Fatal("expected isolated delete contention", err)
		}
		var prompt bool
		if err := db.QueryRow("SELECT next_attempt_at<now()+interval '3 seconds' AND error_code='' FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", p.ID, panel).Scan(&prompt); err != nil || !prompt {
			t.Fatal("busy delete became a blocked failure", prompt, err)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
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

	// A provider can confirm server removal while a native proof is in flight.
	// Neither a late SSH failure nor the outer deadline handler may resurrect it.
	for _, mode := range []string{"ssh_failure", "deadline"} {
		t.Run("provider_retirement_"+mode, func(t *testing.T) {
			sqlMust(t, db, "UPDATE droplets SET state='READY',expires_at=now()+interval '5 minutes' WHERE id=$1", droplet)
			sqlMust(t, db, "UPDATE deployments SET host='127.0.0.1' WHERE droplet_id=$1", droplet)
			sqlMust(t, db, "UPDATE config_driver_listeners SET state='VERIFIED',stage='runtime_verified',error_code='',remote_id=$3,applied_revision=$4,needs_reconcile=false,mutation_state='CONFIRMED',lease_until=NULL,claim_token=NULL,next_attempt_at=now()-interval '1 second' WHERE profile_id=$1 AND panel_id=$2", sibling.ID, panel, siblingRemote, sibling.Revision)
			var credentialBefore string
			if e := db.QueryRow("SELECT credential_ref FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", sibling.ID, panel).Scan(&credentialBefore); e != nil {
				t.Fatal(e)
			}
			batch.beforeReturn = func(proofContext context.Context) error {
				sqlMust(t, db, "UPDATE droplets SET state='DELETED' WHERE id=$1", droplet)
				sqlMust(t, db, "UPDATE resources SET state='deleted' WHERE account_id=(SELECT account_id FROM droplets WHERE id=$1) AND provider_resource_id=(SELECT provider_resource_id FROM droplets WHERE id=$1)", droplet)
				if e := svc.RunOne(ctx); e != nil {
					t.Fatal("provider retirement", e)
				}
				if mode == "deadline" {
					<-proofContext.Done()
					return proofContext.Err()
				}
				return errors.New("native proof lost during confirmed provider retirement")
			}
			proofContext, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			batchService.Verify = nil
			e := batchService.RunVerifyOne(proofContext)
			if mode == "deadline" && !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("deadline not exercised", e)
			}
			if mode == "ssh_failure" && e != nil {
				t.Fatal(e)
			}
			assertRetired := func() {
				t.Helper()
				var safe bool
				e := db.QueryRow("SELECT state='RETIRED' AND stage='server_removed' AND error_code='' AND remote_id IS NULL AND verified_at IS NULL AND applied_revision=0 AND mutation_state='CONFIRMED' AND claim_token IS NULL AND lease_until IS NULL AND credential_ref=$3 FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", sibling.ID, panel, credentialBefore).Scan(&safe)
				if e != nil || !safe {
					t.Fatal("late proof overwrote provider tombstone", mode, safe, e)
				}
			}
			assertRetired()
			// Existing inconsistent tombstones from an older worker self-repair only
			// after the same provider deletion / managed resource absence evidence.
			sqlMust(t, db, "UPDATE config_driver_listeners SET stage='runtime_retry',error_code='profile_verification_timeout' WHERE profile_id=$1 AND panel_id=$2", sibling.ID, panel)
			if e := svc.RunOne(ctx); e != nil {
				t.Fatal(e)
			}
			assertRetired()
			batch.beforeReturn = nil
		})
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

	t.Run("snapshot batch deletion isolates edits and retries", func(t *testing.T) {
		sibling := p
		sibling.RequestID = ""
		sibling.Name = "Sibling"
		code, sibling = profileSave(t, api, sibling, "POST")
		if code != 200 {
			t.Fatal(code)
		}
		request := []profileDeleteItem{{first.ID, first.Revision}, {sibling.ID, sibling.Revision}, {"00000000-0000-0000-0000-000000000001", 1}}
		sibling.Name = "Concurrent edit"
		code, sibling = profileSave(t, api, sibling, "PUT")
		if code != 200 {
			t.Fatal(code)
		}
		late := p
		late.RequestID = ""
		late.Name = "Created after selection"
		code, late = profileSave(t, api, late, "POST")
		if code != 200 {
			t.Fatal(code)
		}
		body, _ := json.Marshal(map[string]any{"profiles": request})
		for attempt := 0; attempt < 2; attempt++ {
			res := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), principalKey{}, auth.Principal{Username: "bulk-delete-admin", Role: auth.Admin}))
			api.deleteConfigProfiles(res, req)
			var reply struct{ Accepted, Conflicts int }
			json.Unmarshal(res.Body.Bytes(), &reply)
			if res.Code != 202 || reply.Accepted != 1 || reply.Conflicts != 2 {
				t.Fatal("partial deletion", res.Code, res.Body.String())
			}
		}
		rows, e := db.Query(`SELECT actor,result,metadata FROM audit_events WHERE action='config_profiles_delete_requested' ORDER BY id`)
		if e != nil {
			t.Fatal(e)
		}
		attempts := 0
		for rows.Next() {
			var actor, result string
			var raw []byte
			if e = rows.Scan(&actor, &result, &raw); e != nil {
				t.Fatal(e)
			}
			var audit struct {
				Profiles            []profileDeleteItem
				Accepted, Conflicts int
				NewIntents          int `json:"new_intents"`
			}
			if e = json.Unmarshal(raw, &audit); e != nil || actor != "bulk-delete-admin" || result != "partial" || !reflect.DeepEqual(audit.Profiles, request) || audit.Accepted != 1 || audit.Conflicts != 2 || audit.NewIntents != 1-attempts {
				t.Fatal("audit lost actor, frozen scope, or idempotent result", actor, result, string(raw), e)
			}
			attempts++
		}
		if e = rows.Err(); e != nil || attempts != 2 {
			t.Fatal("missing audit attempt", attempts, e)
		}
		rows.Close()
		for _, id := range []string{sibling.ID, late.ID} {
			current, e := configplane.Read(context.Background(), db, id)
			if e != nil || current.DeleteRequestedAt != nil || !current.Enabled {
				t.Fatal("unselected or edited profile removed", id, e)
			}
		}
		var events int
		if e := db.QueryRow("SELECT count(*) FROM config_driver_events WHERE profile_id=$1 AND stage='delete_requested'", first.ID).Scan(&events); e != nil || events != 1 {
			t.Fatal("retry duplicated deletion intent", events, e)
		}
	})
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
	sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,remote_id,credential_ref,applied_spec,applied_revision,state,verified_at,mutation_state) VALUES($1,$2,'fixture',20,'ref',$3,1,'VERIFIED',now(),'CONFIRMED')", p.ID, x.Panel, raw)
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
	readServer := func() map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("id", p.ID)
		api.configProfileServers(w, r)
		var d struct{ Servers []map[string]any }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil || len(d.Servers) != 1 {
			t.Fatal("server publication status", w.Code, w.Body.String())
		}
		return d.Servers[0]
	}
	item := read()
	if item["ready"] != float64(0) || item["published"] != float64(0) || item["target"] != float64(2) {
		t.Fatal("verified listener was incorrectly counted as published", item)
	}
	sqlMust(t, db, "INSERT INTO config_driver_class_proofs(profile_id,panel_id,route_class,verified_at) VALUES($1,$2,'DIRECT',now())", p.ID, x.Panel)
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

	server := readServer()
	if server["published"] != float64(2) || server["publication_target"] != float64(2) || server["publication_code"] != "" {
		t.Fatal("server status diverged from actual output", server)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET verified_at=now()-interval '61 seconds' WHERE panel_id=$1", x.Panel)
	server = readServer()
	if server["published"] != float64(0) || server["publication_code"] != "profile_routes_refresh_pending" {
		t.Fatal("stale route labeled published", server)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET verified_at=now() WHERE panel_id=$1", x.Panel)
	sqlMust(t, db, "INSERT INTO config_driver_clients(profile_id,panel_id,client_id,email,route_class,confirmed) VALUES($1,$2,'pending-reservation','pending-email','DIRECT',false)", p.ID, x.Panel)
	item = read()
	stats := item["class_stats"].(map[string]any)["DIRECT"].(map[string]any)
	if stats["active"] != float64(2) || item["ready"] != float64(1) {
		t.Fatal("unconfirmed reservation counted as native creation", item)
	}
	sqlMust(t, db, "UPDATE output_config_snapshots SET last_seen_at=now()-interval '20 seconds' WHERE config_profile_id=$1", p.ID)
	item = read()
	if item["ready"] != float64(0) || item["published"] != float64(0) || item["verified"] != float64(1) {
		t.Fatal("stale output counted as ready", item)
	}
	server = readServer()
	if server["published"] != float64(0) || server["publication_code"] != "profile_output_refresh_pending" {
		t.Fatal("stale snapshot labeled published", server)
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", x.Account)
	item = read()
	if item["eligible"] != float64(0) || item["target"] != float64(0) {
		t.Fatal("disabled account counted as eligible", item)
	}

}

// No credentials enter command arguments. The fixture returns per-profile and
// per-class outcomes exactly like the encrypted remote verifier.
type profileBatchFixtureSSH struct {
	beforeReturn func(context.Context) error
	oldCore      bool
	cancel       context.CancelFunc
	failed       string
	jobs         int
}

func (s *profileBatchFixtureSSH) Run(context.Context, provisioning.Target, []byte, string) (string, error) {
	return "", errors.New("unexpected argv request")
}
func (s *profileBatchFixtureSSH) RunInput(ctx context.Context, _ provisioning.Target, _ []byte, _ string, input []byte) (provisioning.CommandResult, error) {
	if s.cancel != nil {
		s.cancel()
		return provisioning.CommandResult{}, ctx.Err()
	}
	var jobs []struct {
		ProfileID string `json:"profile_id"`
	}
	if e := json.Unmarshal(input, &jobs); e != nil {
		return provisioning.CommandResult{}, e
	}
	s.jobs = len(jobs)
	if s.beforeReturn != nil {
		if e := s.beforeReturn(ctx); e != nil {
			return provisioning.CommandResult{}, e
		}
	}
	out := []any{}
	for _, j := range jobs {
		ok := j.ProfileID != s.failed
		reason := ""
		if !ok {
			reason = "users"
		}
		out = append(out, map[string]any{"profile_id": j.ProfileID, "core_current": !s.oldCore, "verified": ok, "reason": reason, "classes": map[string]bool{"DIRECT": ok, "RESIDENTIAL": false}})
	}
	raw, _ := json.Marshal(map[string]any{"results": out})
	return provisioning.CommandResult{Stdout: string(raw)}, nil
}
