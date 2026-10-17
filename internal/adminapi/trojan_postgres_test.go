package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTrojanMixedOutputKeepsRealityAndScope(t *testing.T) {
	db := adminTestDB(t)
	fixtures := seedAccountOutput(t, db)
	s := &Server{DB: db}
	x := fixtures[0]
	sqlMust(t, db, "UPDATE droplets SET profile='{\"Region\":\"fra1\"}' WHERE id=$1", x.Droplet)
	sqlMust(t, db, "UPDATE trojan_tcp_policy SET enabled=true,fleet=true")
	sqlMust(t, db, `INSERT INTO trojan_tcp_listeners(panel_id,port,remote_id,managed_key,credential_ref,policy_revision,state,certificate_not_after,verified_at) VALUES($1,8443,7,'dob:trojan-tcp:08443','fixture',1,'VERIFIED',now()+interval '5 days',now())`, x.Panel)
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		id := "trojan-" + class
		sqlMust(t, db, "INSERT INTO trojan_tcp_clients(panel_id,route_class,client_id,email) VALUES($1,$2,$3,$3)", x.Panel, class, id)
		sqlMust(t, db, `INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) SELECT $1,$2,$2,$3,$3,revision FROM residential_routing_control`, x.Panel, id, class)
		uri, e := export.TrojanTCPURI(export.TrojanTCP{Host: "192.0.2.1", Port: 8443, Password: strings.Repeat(class, 16), Remark: "old"})
		if e != nil {
			t.Fatal(e)
		}
		sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,client_id,uri) VALUES($1,$2,$3)", x.Panel, id, uri)
	}
	count := func(w *httptest.ResponseRecorder, vr, tr int) {
		t.Helper()
		if w.Code != 200 {
			t.Fatal("output status", w.Code, w.Body.String())
		}
		a, b := 0, 0
		for _, line := range strings.Fields(w.Body.String()) {
			u, e := url.Parse(line)
			if e != nil {
				t.Fatal(e)
			}
			if u.Scheme == "vless" {
				a++
			} else if u.Scheme == "trojan" {
				b++
				if u.Fragment != "🇩🇪 Germany" || u.Port() != "8443" {
					t.Fatal("Trojan country/port")
				}
			} else {
				t.Fatal("unexpected scheme")
			}
		}
		if a != vr || b != tr {
			t.Fatalf("counts VLESS=%d Trojan=%d expected %d/%d", a, b, vr, tr)
		}
	}
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		w := httptest.NewRecorder()
		s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), class)
		count(w, 4, 1)
		token := accountOutputToken(t, s, x.Account, class)
		count(readAccountOutput(s, token, ""), 1, 1)
		count(readAccountOutput(s, accountOutputToken(t, s, fixtures[1].Account, class), ""), 1, 0)
		public := "trojan-test-" + class
		sqlMust(t, db, "INSERT INTO output_share_tokens(token,route_class) VALUES($1,$2)", public, class)
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("token", public)
		w = httptest.NewRecorder()
		s.sharedOutput(w, r)
		count(w, 4, 1)
	}
	for _, mutation := range []string{
		"UPDATE trojan_tcp_listeners SET state='BLOCKED'",
		"UPDATE trojan_tcp_listeners SET verified_at=now()-interval '6 minutes'",
		"UPDATE trojan_tcp_listeners SET certificate_not_after=now()+interval '4 minutes'",
		"UPDATE trojan_tcp_policy SET enabled=false",
	} {
		sqlMust(t, db, "UPDATE trojan_tcp_policy SET enabled=true")
		sqlMust(t, db, "UPDATE trojan_tcp_listeners SET state='VERIFIED',verified_at=now(),certificate_not_after=now()+interval '5 days'")
		sqlMust(t, db, mutation)
		w := httptest.NewRecorder()
		s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "ALL")
		count(w, 10, 0)
	}
	sqlMust(t, db, "UPDATE trojan_tcp_policy SET enabled=true")
	sqlMust(t, db, "UPDATE trojan_tcp_listeners SET state='VERIFIED',verified_at=now(),certificate_not_after=now()+interval '5 days'")
	w := httptest.NewRecorder()
	s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "ALL")
	count(w, 10, 2)
}
func TestTrojanPolicyPortsCleanupAndRealityPreservation(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://fixture")
	s := &Server{DB: db}
	sqlMust(t, db, "INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound) VALUES('DIRECT','[443,9443]',1)")
	var before string
	if e := db.QueryRow("SELECT row_to_json(p)::text FROM reality_config_profiles p").Scan(&before); e != nil {
		t.Fatal(e)
	}
	lastResponse := ""
	save := func(p trojantcp.Policy) int {
		raw, _ := json.Marshal(p)
		w := httptest.NewRecorder()
		s.putTrojanConfig(w, httptest.NewRequest("PUT", "/", bytes.NewReader(raw)))
		lastResponse = w.Body.String()
		return w.Code
	}
	p := trojantcp.Policy{Enabled: true, Port: 9443, Direct: true, Residential: true, Fleet: true, Revision: 1}
	if save(p) != 409 {
		t.Fatal("Reality reserved port accepted")
	}
	p.Port = 8443
	if save(p) != 200 {
		t.Fatal("independent port rejected", lastResponse)
	}
	if save(p) != 409 {
		t.Fatal("stale policy accepted")
	}
	if _, e := db.Exec("UPDATE reality_config_profiles SET ports='[443,8443]'"); e == nil {
		t.Fatal("reverse port collision accepted")
	}
	if _, e := db.Exec("INSERT INTO global_config_policies(policy_key,ports) VALUES('reality','[8443]') ON CONFLICT(policy_key) DO UPDATE SET ports='[8443]'"); e == nil {
		t.Fatal("global Reality reverse collision accepted")
	}
	var after string
	if e := db.QueryRow("SELECT row_to_json(p)::text FROM reality_config_profiles p").Scan(&after); e != nil || before != after {
		t.Fatal("Reality policy changed", e)
	}
	sqlMust(t, db, `INSERT INTO trojan_tcp_listeners(panel_id,port,managed_key,credential_ref,policy_revision) VALUES($1,8443,'dob:trojan-tcp:08443','fixture',2)`, panel)
	p.Revision = 2
	p.Port = 10443
	if save(p) != 409 {
		t.Fatal("existing listener moved")
	}
	p.Port = 8443
	p.Direct = false
	if save(p) != 409 {
		t.Fatal("existing users removed")
	}
	if _, e := (cleanup.Store{DB: db}).Start(context.Background(), []string{panel}); e != nil {
		t.Fatal(e)
	}
	current, e := trojantcp.ReadPolicy(context.Background(), db)
	if e != nil || current.Enabled {
		t.Fatal("cleanup left Trojan creation enabled", e)
	}
	current.Enabled = true
	if save(current) != 409 {
		t.Fatal("Trojan restarted during unfinished cleanup")
	}
}
