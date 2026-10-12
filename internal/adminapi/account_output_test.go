package adminapi

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type accountOutputFixture struct {
	Account, Droplet, Panel string
	URIs                    map[string]string
}

func seedAccountOutput(t *testing.T, db *sql.DB) []accountOutputFixture {
	t.Helper()
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	proxy := perfUUID()
	sqlMust(t, db, "INSERT INTO proxies(id,name,type,host,port,status,last_success_at) VALUES($1,'fixture','socks5','localhost',1080,'healthy',now())", proxy)
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'fixture','socks5','localhost',1080,'healthy',now(),'residential-fixture')", proxy)
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		sqlMust(t, db, "INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound) VALUES($1,'[443]',1)", class)
	}
	var rev int64
	if err := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev); err != nil {
		t.Fatal(err)
	}
	var fixtures []accountOutputFixture
	for i, provider := range []string{"digitalocean", "vultr", "upcloud", "linode"} {
		a, d, p := seedPanel(t, db, "http://fixture.invalid")
		sqlMust(t, db, "UPDATE accounts SET provider=$2,name='Same account name' WHERE id=$1", a, provider)
		sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,state,verified_at,selected_proxy_id) VALUES($1,$2,'APPLIED',now(),$3)", p, rev, proxy)
		var gen string
		if err := db.QueryRow("INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,1,'POLICY',$2) RETURNING id::text", p, "account-output-"+p).Scan(&gen); err != nil {
			t.Fatal(err)
		}
		f := accountOutputFixture{a, d, p, map[string]string{}}
		for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
			id := "client-" + class + "-" + p
			uri := fmt.Sprintf("vless://%s@192.0.2.%d:443?security=reality&encryption=none&sni=example.com#exact%%20%s", id, i+1, class)
			f.URIs[class] = uri
			sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) VALUES($1,$2,$2,$3,$3,$4)", p, id, class, rev)
			sqlMust(t, db, "INSERT INTO bulk_user_ownership(generation_id,client_id,email,route_class,state) VALUES($1,$2,$2,$3,'ACTIVE')", gen, id, class)
			sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id,visible_until) VALUES($1,$2,$3,now()+interval '30 minutes')", p, uri, id)
		}
		fixtures = append(fixtures, f)
	}
	// A copied client identity on another panel has no local immutable ownership.
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		id := "client-" + class + "-" + fixtures[0].Panel
		sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) VALUES($1,$2,$2,$3,$3,$4)", fixtures[1].Panel, id, class, rev)
		sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id) VALUES($1,$2,$3)", fixtures[1].Panel, "vless://"+id+"@192.0.2.200:443#foreign-owner", id)
	}
	return fixtures
}
func accountOutputToken(t *testing.T, s *Server, id, class string) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"route_class":"`+class+`"}`))
	r.SetPathValue("id", id)
	s.createAccountOutputShare(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var x map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	if x["url"] != "/share/account-output/"+x["token"] {
		t.Fatal(x)
	}
	return x["token"]
}
func readAccountOutput(s *Server, token, query string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/"+query, nil)
	r.SetPathValue("token", token)
	s.sharedAccountOutput(w, r)
	return w
}
func TestAccountOutputIsolationAndCompatibility(t *testing.T) {
	db := adminTestDB(t)
	f := seedAccountOutput(t, db)
	s := &Server{DB: db}
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		global := httptest.NewRecorder()
		s.outputSnapshotClass(global, httptest.NewRequest("GET", "/", nil), class)
		if global.Code != 200 || len(strings.Fields(global.Body.String())) != 4 {
			t.Fatal("global regression", global.Code, global.Body.String())
		}
		for _, x := range f {
			token := accountOutputToken(t, s, x.Account, class)
			for _, q := range []string{"", "?created_within_minutes=1", "?expires_within_minutes=120", "?created_within_minutes=1&expires_within_minutes=120"} {
				w := readAccountOutput(s, token, q)
				if w.Code != 200 || w.Body.String() != x.URIs[class]+"\n" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("scope/raw URI", w.Code, w.Body.String())
				}
			}
			for _, q := range []string{"?account_id=" + f[1].Account, "?account_id=", "?route_class=ALL", "?route_class=" + class + "&route_class=ALL", "?view=1&account_id=" + f[1].Account} {
				w := readAccountOutput(s, token, q)
				if w.Code != 400 || strings.Contains(w.Body.String(), "vless://") {
					t.Fatal("tampering", q, w.Code)
				}
			}
			legacy := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.SetPathValue("token", token)
			s.sharedOutput(legacy, req)
			if legacy.Code != 404 {
				t.Fatal("scoped token accepted by global reader", legacy.Code)
			}
		}
	}
	// Calls overlap with repeated, fixed capability reads; no shared mutable scope.
	var wg sync.WaitGroup
	for _, x := range f {
		for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
			token := accountOutputToken(t, s, x.Account, class)
			want := x.URIs[class] + "\n"
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 5; i++ {
					w := readAccountOutput(s, token, "")
					if w.Code != 200 || w.Body.String() != want {
						t.Error("concurrent scope mismatch", w.Code)
					}
				}
			}()
		}
	}
	wg.Wait()
}
func TestAccountOutputEligibilityAndRevocation(t *testing.T) {
	db := adminTestDB(t)
	f := seedAccountOutput(t, db)
	s := &Server{DB: db}
	x := f[0]
	token := accountOutputToken(t, s, x.Account, "DIRECT")
	checks := []struct {
		alter, restore string
		args           []any
	}{
		{"UPDATE output_config_snapshots SET visible_until=now()-interval '1 second' WHERE panel_id=$1", "UPDATE output_config_snapshots SET visible_until=now()+interval '30 minutes' WHERE panel_id=$1", []any{x.Panel}},
		{"UPDATE output_config_snapshots SET last_seen_at=now()-interval '16 seconds' WHERE panel_id=$1", "UPDATE output_config_snapshots SET last_seen_at=now() WHERE panel_id=$1", []any{x.Panel}},
		{"UPDATE droplets SET expires_at=now()+interval '9 seconds' WHERE id=$1", "UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id=$1", []any{x.Droplet}},
		{"UPDATE droplets SET state='RETIRING' WHERE id=$1", "UPDATE droplets SET state='READY' WHERE id=$1", []any{x.Droplet}},
		{"UPDATE panel_routing_state SET verified_at=now()-interval '61 seconds' WHERE panel_id=$1", "UPDATE panel_routing_state SET verified_at=now() WHERE panel_id=$1", []any{x.Panel}},
		{"UPDATE panel_instances SET account_id=$2 WHERE id=$1", "UPDATE panel_instances SET account_id=$3 WHERE id=$1", []any{x.Panel, f[1].Account, x.Account}},
		{"UPDATE deployments SET account_id=$2 WHERE droplet_id=$1", "UPDATE deployments SET account_id=$3 WHERE droplet_id=$1", []any{x.Droplet, f[1].Account, x.Account}},
	}
	for i, c := range checks {
		// Parameter lists match each statement, including intentional owner mismatch.
		if len(c.args) == 3 {
			sqlMust(t, db, c.alter, c.args[0], c.args[1])
		} else {
			sqlMust(t, db, c.alter, c.args...)
		}
		w := readAccountOutput(s, token, "")
		if w.Code != 200 || w.Body.Len() != 0 {
			t.Fatal("eligibility", i, w.Code, w.Body.String())
		}
		if len(c.args) == 3 {
			sqlMust(t, db, strings.Replace(c.restore, "$3", "$2", 1), c.args[0], c.args[2])
		} else {
			sqlMust(t, db, c.restore, c.args...)
		}
		// Lifecycle transitions intentionally purge snapshots; a fresh worker observation
		// is required after restoring eligibility. Recreate only this fixture's evidence.
		for class, uri := range x.URIs {
			sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id,visible_until) VALUES($1,$2,$3,now()+interval '30 minutes') ON CONFLICT(panel_id,uri) DO UPDATE SET last_seen_at=now(),visible_until=excluded.visible_until", x.Panel, uri, "client-"+class+"-"+x.Panel)
		}
		if w := readAccountOutput(s, token, ""); w.Code != 200 || w.Body.String() != x.URIs["DIRECT"]+"\n" {
			t.Fatal("eligibility restore", i, w.Code, w.Body.String())
		}
	}
	res := accountOutputToken(t, s, x.Account, "RESIDENTIAL")
	sqlMust(t, db, "UPDATE residential_proxies SET last_success_at=now()-interval '4 minutes'")
	if w := readAccountOutput(s, res, ""); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("residential unhealthy", w.Code)
	}
	if w := readAccountOutput(s, token, ""); w.Code != 200 || w.Body.String() != x.URIs["DIRECT"]+"\n" {
		t.Fatal("direct impacted")
	}
	sqlMust(t, db, "UPDATE account_output_share_tokens SET revoked_at=now() WHERE token=$1", token)
	if w := readAccountOutput(s, token, ""); w.Code != 404 {
		t.Fatal("revoked", w.Code)
	}
	// Simulate revocation after the handler's capability lookup.
	w := httptest.NewRecorder()
	s.outputSnapshotScoped(w, httptest.NewRequest("GET", "/", nil), "DIRECT", &accountOutputScope{AccountID: x.Account, Token: token})
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("revocation lookup/query race", w.Code, w.Body.String())
	}
	sqlMust(t, db, "UPDATE accounts SET deleted_at=now() WHERE id=$1", x.Account)
	if w := readAccountOutput(s, res, "?view=1"); w.Code != 404 {
		t.Fatal("soft delete", w.Code)
	}
}
func TestAccountOutputCreationDeletionAndFaults(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	id := perfUUID()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'empty','linode','fixture')", id)
	for _, test := range []struct {
		id, body string
		code     int
	}{
		{"bad", `{"route_class":"DIRECT"}`, 400}, {perfUUID(), `{"route_class":"DIRECT"}`, 404},
		{id, "", 400}, {id, `{}`, 400}, {id, `{"route_class":"ALL"}`, 400}, {id, `{"route_class":"DIRECT","account_id":"other"}`, 400}, {id, `{"route_class":"DIRECT"}{}`, 400},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
		r.SetPathValue("id", test.id)
		s.createAccountOutputShare(w, r)
		if w.Code != test.code {
			t.Fatal(test, w.Code, w.Body.String())
		}
	}
	token := accountOutputToken(t, s, id, "DIRECT")
	if w := readAccountOutput(s, token, ""); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("empty account fallback", w.Code, w.Body.String())
	}
	sqlMust(t, db, "DELETE FROM accounts WHERE id=$1", id)
	if w := readAccountOutput(s, token, ""); w.Code != 404 {
		t.Fatal("deleted token", w.Code)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM account_output_share_tokens WHERE token=$1", token).Scan(&n); err != nil || n != 0 {
		t.Fatal("cascade", n, err)
	}
	for _, scope := range []*accountOutputScope{{}, {AccountID: id}, {Token: token}} {
		w := httptest.NewRecorder()
		s.outputSnapshotScoped(w, httptest.NewRequest("GET", "/", nil), "DIRECT", scope)
		if w.Code != 400 {
			t.Fatal("invalid scope broadened", w.Code)
		}
	}
	db.Close()
	if w := readAccountOutput(s, token, ""); w.Code != 500 || strings.Contains(w.Body.String(), "vless://") {
		t.Fatal("DB fault", w.Code)
	}
}

func TestAccountOutputAuthenticatedRoutes(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db, Auth: auth.Service{Store: auth.SQLStore{DB: db}}}
	id := perfUUID()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','linode','fixture')", id)
	for _, role := range []auth.Role{auth.Admin, auth.Operator, auth.Viewer} {
		user := perfUUID()
		token := "fixture-" + string(role)
		hash := sha256.Sum256([]byte(token))
		sqlMust(t, db, "INSERT INTO admin_users(id,username,password_hash,role,enabled) VALUES($1,$2,'fixture',$2,true)", user, string(role))
		sqlMust(t, db, "INSERT INTO admin_sessions(id,user_id,token_hash,expires_at) VALUES(gen_random_uuid(),$1,$2,now()+interval '1 hour')", user, hash[:])
	}
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 401}, {"bad", 401}, {"fixture-viewer", 403}, {"fixture-operator", 200}, {"fixture-admin", 200}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/accounts/"+id+"/output/share", strings.NewReader(`{"route_class":"DIRECT"}`))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		s.Routes().ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
}
func TestAccountOutputIncludesEveryOwnedPanel(t *testing.T) {
	db := adminTestDB(t)
	f := seedAccountOutput(t, db)
	s := &Server{DB: db}
	// Construct a two-server account before issuing its capability.
	sqlMust(t, db, "UPDATE droplets SET account_id=$2 WHERE id=$1", f[1].Droplet, f[0].Account)
	sqlMust(t, db, "UPDATE panel_instances SET account_id=$2 WHERE id=$1", f[1].Panel, f[0].Account)
	sqlMust(t, db, "UPDATE deployments SET account_id=$2 WHERE droplet_id=$1", f[1].Droplet, f[0].Account)
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		token := accountOutputToken(t, s, f[0].Account, class)
		w := readAccountOutput(s, token, "")
		got := strings.Fields(w.Body.String())
		if w.Code != 200 || len(got) != 2 || !strings.Contains(w.Body.String(), f[0].URIs[class]+"\n") || !strings.Contains(w.Body.String(), f[1].URIs[class]+"\n") {
			t.Fatal("multiple panels", w.Code, w.Body.String())
		}
		other := accountOutputToken(t, s, f[1].Account, class)
		w = readAccountOutput(s, other, "")
		if w.Code != 200 || w.Body.Len() != 0 {
			t.Fatal("empty account", w.Code, w.Body.String())
		}
	}
}
