package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProfileOutputRevisionScopeAndCountries(t *testing.T) {
	db := adminTestDB(t)
	fixtures := seedAccountOutput(t, db)
	x := fixtures[0]
	s := &Server{DB: db}
	sqlMust(t, db, "UPDATE droplets SET profile='{\"Region\":\"fra1\"}' WHERE id=$1", x.Droplet)
	types := []string{"vless", "vmess", "trojan", "shadowsocks"}
	ids := []string{}
	uris := []string{}
	for i, protocol := range types {
		id := "00000000-0000-4000-8000-00000000000" + string(rune('1'+i))
		ids = append(ids, id)
		spec := configdriver.Spec{Protocol: protocol, Network: "tcp", Security: "none", Port: 19000 + i}
		if protocol == "vless" {
			spec.Security = "reality"
			spec.SNI = "example.com"
		}
		if protocol == "trojan" {
			spec.HeaderType = "http"
			spec.Host = "play.googleapis.com"
			spec.Path = "/"
		}
		raw, _ := json.Marshal(spec)
		sqlMust(t, db, "INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,classes) VALUES($1,$2,$3,true,true,ARRAY['DIRECT'])", id, protocol, raw)
		sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,remote_id,credential_ref,applied_spec,applied_revision,state,verified_at,certificate_not_after) VALUES($1,$2,$3,$4,'fixture',$5,1,'VERIFIED',now(),now()+interval '5 days')", id, x.Panel, "dob:profile:"+id, i+20, raw)
		sqlMust(t, db, "INSERT INTO config_driver_class_proofs(profile_id,panel_id,route_class,verified_at) VALUES($1,$2,'DIRECT',now())", id, x.Panel)
		creds, e := configdriver.NewCredentials(id, x.Panel, []string{"DIRECT"}, 1)
		if e != nil {
			t.Fatal(e)
		}
		c := creds.Clients[0]
		sqlMust(t, db, "INSERT INTO config_driver_clients(profile_id,panel_id,client_id,email,route_class) VALUES($1,$2,$3,$4,'DIRECT')", id, x.Panel, c.ID, c.Email)
		sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) SELECT $1,$2,$3,'DIRECT','DIRECT',revision FROM residential_routing_control", x.Panel, c.ID, c.Email)
		uri, e := configdriver.Builtins().URI(spec, "192.0.2.1", "old-name", creds, c)
		if e != nil {
			t.Fatal(e)
		}
		uris = append(uris, uri)
		sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,client_id,uri,config_profile_id,config_profile_revision) VALUES($1,$2,$3,$4,1)", x.Panel, c.ID, uri, id)
	}
	read := func(class string) []string {
		t.Helper()
		w := httptest.NewRecorder()
		s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), class)
		if w.Code != 200 {
			t.Fatal("output status", w.Code, w.Body.String())
		}
		return strings.Fields(w.Body.String())
	}
	check := func(lines []string, want int) {
		t.Helper()
		seen := 0
		for _, uri := range lines {
			port, e := export.URIPort(uri)
			if e != nil {
				t.Fatal(e)
			}
			if port < 19000 || port > 19003 {
				continue
			}
			seen++
			if strings.HasPrefix(uri, "vmess://") {
				v, e := export.DecodeVMess(uri)
				if e != nil || v["ps"] != "🇩🇪 Germany" {
					t.Fatal("VMess name")
				}
			} else {
				u, e := url.Parse(uri)
				if e != nil || u.Fragment != "🇩🇪 Germany" {
					t.Fatal("name")
				}
			}
		}
		if seen != want {
			t.Fatal("profile count", seen, want)
		}
	}
	check(read("ALL"), 4)
	check(read("DIRECT"), 4)
	check(read("RESIDENTIAL"), 0)
	token := accountOutputToken(t, s, fixtures[1].Account, "DIRECT")
	check(strings.Fields(readAccountOutput(s, token, "").Body.String()), 0)
	token = accountOutputToken(t, s, x.Account, "DIRECT")
	check(strings.Fields(readAccountOutput(s, token, "").Body.String()), 4)
	// An invalid owned profile is withheld, without poisoning valid siblings.
	bad := strings.Replace(uris[2], "headerType=http", "headerType=unknown", 1)
	sqlMust(t, db, "UPDATE output_config_snapshots SET uri=$2 WHERE config_profile_id=$1", ids[2], bad)
	w := httptest.NewRecorder()
	s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "DIRECT")
	if w.Code != 200 || w.Header().Get("X-Output-Withheld-Count") != "1" {
		t.Fatal("invalid profile poisoned output", w.Code)
	}
	check(strings.Fields(w.Body.String()), 3)
	check(strings.Fields(readAccountOutput(s, token, "").Body.String()), 3)
	sqlMust(t, db, "UPDATE output_config_snapshots SET uri=$2 WHERE config_profile_id=$1", ids[2], uris[2])
	// A fresh snapshot from the previous revision must not leak during/after apply.
	sqlMust(t, db, "UPDATE config_driver_profiles SET revision=2 WHERE id=$1", ids[0])
	check(read("ALL"), 3)
	sqlMust(t, db, "UPDATE config_driver_listeners SET applied_revision=2 WHERE profile_id=$1", ids[0])
	check(read("ALL"), 3)
	sqlMust(t, db, "UPDATE output_config_snapshots SET config_profile_revision=2 WHERE config_profile_id=$1", ids[0])
	check(read("ALL"), 4)
	sqlMust(t, db, "UPDATE config_driver_profiles SET enabled=false WHERE id=$1", ids[1])
	check(read("ALL"), 3)
	sqlMust(t, db, "UPDATE config_driver_listeners SET verified_at=now()-interval '6 minutes' WHERE profile_id=$1", ids[2])
	check(read("ALL"), 2)
	sqlMust(t, db, "DELETE FROM config_driver_listeners WHERE profile_id=$1", ids[3])
	check(read("ALL"), 1)
	// No mutation of stored connection strings during country display formatting.
	for _, uri := range uris {
		var same bool
		if e := db.QueryRow("SELECT EXISTS(SELECT 1 FROM output_config_snapshots WHERE uri=$1)", uri).Scan(&same); e != nil || !same {
			t.Fatal("stored URI changed")
		}
	}
}
