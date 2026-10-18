package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func profileSave(t *testing.T, s *Server, p configplane.Profile, method string) (int, configplane.Profile) {
	t.Helper()
	b, _ := json.Marshal(p)
	r := httptest.NewRequest(method, "/", bytes.NewReader(b))
	r.SetPathValue("id", p.ID)
	w := httptest.NewRecorder()
	s.saveConfigProfile(w, r)
	var out struct{ Profile configplane.Profile }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 {
		t.Log("profile response", w.Code, w.Body.String())
	}
	return w.Code, out.Profile
}
func TestConfigDriverProfileAdmission(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	sqlMust(t, db, "INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound) VALUES('DIRECT','[443]',1)")
	p := configplane.Profile{Name: "Independent", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 443}}
	if code, _ := profileSave(t, s, p, "POST"); code != 409 {
		t.Fatal("Reality port collision accepted")
	}
	p.Spec.Port = 0
	code, first := profileSave(t, s, p, "POST")
	if code != 200 || first.Spec.Port < 10000 {
		t.Fatal("automatic reservation", code)
	}
	code, second := profileSave(t, s, p, "POST")
	if code != 200 || second.Spec.Port == first.Spec.Port {
		t.Fatal("reservation duplicated")
	}
	second.Spec.Port = first.Spec.Port
	if code, _ := profileSave(t, s, second, "PUT"); code != 409 {
		t.Fatal("profile conflict accepted")
	}
	first.Name = "Renamed"
	code, updated := profileSave(t, s, first, "PUT")
	if code != 200 || updated.Revision != 2 {
		t.Fatal("save failed")
	}
	if code, _ := profileSave(t, s, first, "PUT"); code != 409 {
		t.Fatal("stale save accepted")
	}
	p.Spec = configdriver.Spec{Protocol: "trojan", Network: "tcp", Security: "none", Port: 19000}
	if code, _ := profileSave(t, s, p, "POST"); code != 400 {
		t.Fatal("public plaintext Trojan accepted")
	}
	p.Spec = configdriver.Spec{Protocol: "vless", Network: "ws", Security: "reality", Port: 19000, Path: "/profile", SNI: "example.com"}
	if code, _ := profileSave(t, s, p, "POST"); code != 400 {
		t.Fatal("unsupported combination accepted")
	}
	var n int
	if db.QueryRow("SELECT count(*) FROM config_driver_profiles").Scan(&n) != nil || n != 2 {
		t.Fatal("rejected request persisted")
	}
}
func TestConfigDriverRecoveryAndIndependentPortChange(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	var mu sync.Mutex
	posts, updates := 0, 0
	externalWriter := false
	checkWriter := func(wantAvailable bool) {
		conn, e := db.Conn(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Close()
		var available bool
		if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(628341902731)").Scan(&available); e != nil {
			t.Fatal(e)
		}
		if available {
			if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)"); e != nil {
				t.Fatal(e)
			}
		}
		if available != wantAvailable {
			t.Errorf("shared writer availability=%v, want %v", available, wantAvailable)
		}
	}
	native := map[int]map[string]any{}
	sibling := json.RawMessage(`{"id":1,"tag":"inbound-443","remark":"dob:global-reality:00443","protocol":"vless","port":443,"enable":true,"settings":{"clients":[]},"streamSettings":{"network":"tcp","security":"reality"}}`)
	dropReads := false
	lostOnce := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/csrf-token":
			fmt.Fprint(w, `{"success":true,"obj":"csrf"}`)
		case r.URL.Path == "/login":
			fmt.Fprint(w, `{"success":true}`)
		case r.URL.Path == "/panel/api/inbounds/list":
			if dropReads {
				w.WriteHeader(503)
				fmt.Fprint(w, `{"success":false}`)
				return
			}
			rows := []json.RawMessage{sibling}
			for _, in := range native {
				b, _ := json.Marshal(in)
				rows = append(rows, b)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": rows})
		case r.URL.Path == "/panel/api/clients/list":
			out := []any{}
			for _, n := range native {
				raw, _ := n["settings"].(string)
				var settings struct{ Clients []map[string]any }
				_ = json.Unmarshal([]byte(raw), &settings)
				for _, c := range settings.Clients {
					c["uuid"] = c["id"]
					out = append(out, c)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": out})
		case r.URL.Path == "/panel/api/inbounds/add":
			checkWriter(false)
			posts++
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("body")
			}
			id := posts + 1
			body["id"] = id
			body["tag"] = fmt.Sprint("inbound-", id)
			native[id] = body
			if lostOnce {
				dropReads = true
				lostOnce = false
			}
			w.WriteHeader(503)
			fmt.Fprint(w, `{"success":false}`)
		case strings.HasPrefix(r.URL.Path, "/panel/api/inbounds/update/"):
			checkWriter(false)
			updates++
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("body")
			}
			id := int(body["id"].(float64))
			body["tag"] = fmt.Sprint("inbound-", id)
			native[id] = body
			w.WriteHeader(503)
			fmt.Fprint(w, `{"success":false}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	_, droplet, panel := seedPanel(t, db, server.URL)
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.1' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL")
	p := configplane.Profile{Name: "VMess", Enabled: true, Fleet: true, Classes: []string{"DIRECT", "RESIDENTIAL"}, Count: 1, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 18444}}
	status, p := profileSave(t, &Server{DB: db}, p, "POST")
	if status != 200 {
		t.Fatal(status)
	}
	secrets := &trojanTestSecrets{values: map[string][]byte{}}
	service := func() configplane.Service {
		return configplane.Service{DB: db, Secrets: secrets, SSH: &trojanTestSSH{}, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: time.Second}}, Verify: func(context.Context, provisioning.Target, []byte, map[string]any) error {
			checkWriter(!externalWriter)
			return nil
		}}
	}
	if e := service().Reconcile(ctx, p.ID, panel); e == nil {
		t.Fatal("lost read falsely confirmed")
	}
	var stage string
	if e := db.QueryRow("SELECT mutation_state FROM config_driver_listeners WHERE profile_id=$1", p.ID).Scan(&stage); e != nil || stage != "SENT" {
		t.Fatal(stage, e)
	}
	mu.Lock()
	dropReads = false
	mu.Unlock()
	// A SENT write can be observed while an unrelated writer holds the global
	// lease. It must not POST again, and normal writes must still own that lease.
	writer, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer writer.Close()
	if _, e = writer.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); e != nil {
		t.Fatal(e)
	}
	defer writer.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()")
	externalWriter = true
	recovery, stop := context.WithTimeout(ctx, 3*time.Second)
	e = service().Reconcile(recovery, p.ID, panel)
	stop()
	externalWriter = false
	if _, unlockErr := writer.ExecContext(ctx, "SELECT pg_advisory_unlock(628341902731)"); unlockErr != nil {
		t.Fatal(unlockErr)
	}
	if e != nil {
		t.Fatal("observation recovery blocked behind unrelated writer", e)
	}
	if posts != 1 {
		t.Fatal("ambiguous create repeated")
	}
	before := map[string][]byte{}
	for k, v := range secrets.values {
		before[k] = append([]byte(nil), v...)
	}
	p.Spec.Port = 18445
	status, p = profileSave(t, &Server{DB: db}, p, "PUT")
	if status != 200 {
		t.Fatal("port edit rejected")
	}
	if e := service().Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("port change", e)
	}
	if e := service().Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal("post-restart verify", e)
	}
	if posts != 1 || updates != 1 || !reflect.DeepEqual(before, secrets.values) {
		t.Fatal("port change replaced credentials or duplicated native write")
	}
	var gotPort int
	var state string
	if e := db.QueryRow("SELECT state,(applied_spec->>'port')::integer FROM config_driver_listeners WHERE profile_id=$1", p.ID).Scan(&state, &gotPort); e != nil || state != "VERIFIED" || gotPort != 18445 {
		t.Fatal(state, gotPort, e)
	}
	p.Name = "Display only"
	status, p = profileSave(t, &Server{DB: db}, p, "PUT")
	if status != 200 {
		t.Fatal(status)
	}
	if e := service().Reconcile(ctx, p.ID, panel); e != nil {
		t.Fatal(e)
	}
	if updates != 1 {
		t.Fatal("name change mutated listener")
	}
	// A sibling profile remains separately owned, routed and reserved.
	other := configplane.Profile{Name: "SS", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "shadowsocks", Network: "tcp", Security: "none", Port: 18446}}
	status, other = profileSave(t, &Server{DB: db}, other, "POST")
	if status != 200 {
		t.Fatal(status)
	}
	if e := service().Reconcile(ctx, other.ID, panel); e != nil {
		t.Fatal("sibling", e)
	}
	if posts != 2 {
		t.Fatal("sibling was not independent")
	}
	var count int
	if e := db.QueryRow("SELECT count(*) FROM config_driver_clients WHERE panel_id=$1", panel).Scan(&count); e != nil || count != 3 {
		t.Fatal("ownership", count, e)
	}
	// A stale external edit must never be overwritten by a later profile save.
	mu.Lock()
	native[2]["remark"] = "manually-renamed"
	mu.Unlock()
	p.Spec.Port = 18447
	status, p = profileSave(t, &Server{DB: db}, p, "PUT")
	if status != 200 {
		t.Fatal(status)
	}
	if e := service().Reconcile(ctx, p.ID, panel); e == nil {
		t.Fatal("foreign edit overwritten")
	}
	if updates != 1 {
		t.Fatal("conflicting listener mutated")
	}
}
