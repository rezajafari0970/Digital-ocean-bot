package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func seedPanel(t *testing.T, db *sql.DB, base string) (string, string, string) {
	t.Helper()
	a, _ := sanaei.UUIDv4()
	d, _ := sanaei.UUIDv4()
	p, _ := sanaei.UUIDv4()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,provider_state) VALUES($1,'test','digitalocean','test','ACTIVE')", a)
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at) VALUES($1,$2,$3,'READY',now()+interval '1 hour')", d, a, "test-"+d)
	sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'test','{}')", p, a)
	sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES(gen_random_uuid(),$1,$3,$2,'PANEL_COMPLETE','done')", a, d, p)
	sqlMust(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui',$4,'test')", p, a, d, base)
	sqlMust(t, db, "INSERT INTO xui_panel_deployments(id,account_id,droplet_id,username,password_secret_ref,port,web_path,state) VALUES(gen_random_uuid(),$1,$2,'test','test',8080,'/','COMPLETED')", a, d)
	return a, d, p
}
func TestClassOutputDisjointImmutableAndFresh(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := seedPanel(t, db, "http://panel.test")
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	var rev int64
	if err := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,state,verified_at) VALUES($1,$2,'APPLIED',now())", panel, rev)
	proxy, _ := sanaei.UUIDv4()
	sqlMust(t, db, "INSERT INTO proxies(id,name,type,host,port,status,last_success_at) VALUES($1,'test','socks5','localhost',1080,'healthy',now())", proxy)
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'test','socks5','localhost',1080,'healthy',now(),'residential-ads-test')", proxy)
	if err := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET revision=$2,selected_proxy_id=$3 WHERE panel_id=$1", panel, rev, proxy)
	for i, class := range []string{"RESIDENTIAL", "DIRECT", "BLOCKED"} {
		id := fmt.Sprint(i)
		sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) VALUES($1,$2,$2,CASE WHEN $3='DIRECT' THEN 'DIRECT' ELSE 'RESIDENTIAL' END,$3,$4)", panel, id, class, rev)
		sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id) VALUES($1,$2,$3)", panel, "vless://"+id+"@panel.test", id)
	}
	s := Server{DB: db}
	for _, class := range []string{"RESIDENTIAL", "DIRECT"} {
		w := httptest.NewRecorder()
		s.createOutputShare(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"route_class":"`+class+`"}`)))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var share map[string]string
		json.Unmarshal(w.Body.Bytes(), &share)
		req := httptest.NewRequest("GET", "/?created_within_minutes=5&expires_within_minutes=120", nil)
		req.SetPathValue("token", share["token"])
		out := httptest.NewRecorder()
		s.sharedOutput(out, req)
		expected := "vless://0@panel.test"
		if class == "DIRECT" {
			expected = "vless://1@panel.test"
		}
		if out.Code != 200 || strings.TrimSpace(out.Body.String()) != expected {
			t.Fatal(class, out.Code, out.Body.String())
		}
		viewRequest := httptest.NewRequest("GET", "/?view=1", nil)
		viewRequest.SetPathValue("token", share["token"])
		view := httptest.NewRecorder()
		s.sharedOutput(view, viewRequest)
		if view.Code != 200 || !strings.Contains(view.Header().Get("Content-Type"), "text/html") || !strings.Contains(view.Body.String(), "output-live.js") {
			t.Fatal("browser view", view.Code)
		}
		req = httptest.NewRequest("GET", "/?route_class=ALL&view=1", nil)
		req.SetPathValue("token", share["token"])
		out = httptest.NewRecorder()
		s.sharedOutput(out, req)
		if out.Code != 400 {
			t.Fatal("class widened")
		}
	}
	// Health flapping cannot invalidate the fleet revision or DIRECT Output.
	// Residential publication still closes immediately on a failed probe.
	for _, healthy := range []bool{false, true, false, true} {
		status := "down"
		if healthy {
			status = "healthy"
		}
		sqlMust(t, db, "UPDATE residential_proxies SET status=$2,last_success_at=now() WHERE proxy_id=$1", proxy, status)
		var current int64
		if err := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&current); err != nil || current != rev {
			t.Fatal("health changed routing intent", current, rev, err)
		}
		for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
			response := httptest.NewRecorder()
			s.outputSnapshotResponse(response, httptest.NewRequest("GET", "/?route_class="+class, nil))
			want := "vless://1@panel.test"
			if class == "RESIDENTIAL" {
				want = ""
				if healthy {
					want = "vless://0@panel.test"
				}
			}
			if response.Code != 200 || strings.TrimSpace(response.Body.String()) != want {
				t.Fatal("health publication boundary", class, status, response.Code)
			}
		}
	}
	// Historical/corrupt effective state must not move residential identities
	// into the never-residential subscription.
	sqlMust(t, db, "UPDATE panel_client_routes SET effective_class='DIRECT' WHERE panel_id=$1 AND route_class='RESIDENTIAL'", panel)
	directOnly := httptest.NewRecorder()
	s.outputSnapshotResponse(directOnly, httptest.NewRequest("GET", "/?route_class=DIRECT", nil))
	if directOnly.Code != 200 || strings.TrimSpace(directOnly.Body.String()) != "vless://1@panel.test" {
		t.Fatal("residential identity leaked into direct subscription")
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET verified_at=now()-interval '2 minutes'")
	out := httptest.NewRecorder()
	s.outputSnapshotResponse(out, httptest.NewRequest("GET", "/?route_class=DIRECT", nil))
	if out.Code != 200 || out.Body.Len() != 0 {
		t.Fatal("stale route published", out.Code, out.Body.String())
	}
}

type panelTestSecrets struct{}

func (panelTestSecrets) Get(context.Context, string, string) ([]byte, error) {
	return []byte("test"), nil
}
func TestDurableCleanupLostResponseAndWorkerRestart(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	var mu sync.Mutex
	clients, inbound := true, true
	deletes, restarts := 0, 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/csrf-token":
			fmt.Fprint(w, `{"success":true,"obj":"test-csrf"}`)
		case "/login":
			fmt.Fprint(w, `{"success":true}`)
		case "/panel/api/clients/list":
			if clients {
				fmt.Fprint(w, `{"success":true,"obj":[{"email":"manual","uuid":"id-1","inboundIds":[7]}]}`)
			} else {
				fmt.Fprint(w, `{"success":true,"obj":[]}`)
			}
		case "/panel/api/inbounds/list":
			if !inbound {
				fmt.Fprint(w, `{"success":true,"obj":[]}`)
				return
			}
			cs := "[]"
			if clients {
				cs = `[{"email":"manual","id":"id-1"}]`
			}
			fmt.Fprintf(w, `{"success":true,"obj":[{"id":7,"tag":"actual-tag","port":12345,"protocol":"vless","settings":{"clients":%s}}]}`, cs)
		case "/panel/api/clients/bulkDel":
			deletes++
			if !clients {
				t.Error("duplicate delete")
			}
			clients = false
			w.WriteHeader(500)
			fmt.Fprint(w, `{"success":false}`)
		case "/panel/api/inbounds/del/7":
			if clients {
				t.Error("inbound removed before clients")
			}
			restarts++
			inbound = false
			w.WriteHeader(500)
			fmt.Fprint(w, `{"success":false}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	_, _, panel := seedPanel(t, db, remote.URL)
	_, _, other := seedPanel(t, db, "http://untouched.test")
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	id, err := (cleanup.Store{DB: db}).Start(ctx, []string{panel})
	if err != nil {
		t.Fatal(err)
	}
	// No API mutation occurs when the durable request is planned.
	if deletes != 0 {
		t.Fatal("planner mutated")
	}
	for i := 0; i < 4; i++ {
		// Fresh service/runtime simulates worker restart between each bounded chunk.
		svc := cleanup.Service{DB: db, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: panelTestSecrets{}, Timeout: time.Second}}}
		if err = svc.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
	}
	st, err := (cleanup.Store{DB: db}).Status(ctx, id)
	if err != nil || st.Status != "done" || st.Succeeded != 1 || st.Results[0].Deleted != 1 || st.Results[0].DeletedInbounds != 1 {
		t.Fatal(st, err)
	}
	if deletes != 1 || restarts != 1 {
		t.Fatal("duplicate mutation", deletes, restarts)
	}
	var untouched int
	if err = db.QueryRow("SELECT count(*) FROM panel_instances WHERE id=$1", other).Scan(&untouched); err != nil || untouched != 1 {
		t.Fatal("scope escaped")
	}
	var enabled bool
	if err = db.QueryRow("SELECT enabled FROM global_config_policies WHERE policy_key='reality'").Scan(&enabled); err != nil || enabled {
		t.Fatal("creation not frozen", err)
	}
}

func (panelTestSecrets) GetResidential(context.Context, string, string) ([]byte, error) {
	return []byte("test"), nil
}
func TestResidentialWorkerAppliesChangesAndRemoval(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	var mu sync.Mutex
	template := map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}
	running := template
	saves, reloads := 0, 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		result := func(obj any) { json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": obj}) }
		switch r.URL.Path {
		case "/csrf-token":
			result("test")
		case "/login":
			result(nil)
		case "/panel/api/inbounds/list":
			fmt.Fprint(w, `{"success":true,"obj":[{"id":1,"tag":"in-443-tcp","port":443,"protocol":"vless","sniffing":{"enabled":true,"destOverride":["http","tls","quic"]},"settings":{"clients":[{"id":"a","email":"a"},{"id":"b","email":"b"}]}}]}`)
		case "/panel/api/xray/":
			result(map[string]any{"xraySetting": template})
		case "/panel/api/xray/update":
			r.ParseForm()
			var next map[string]any
			if json.Unmarshal([]byte(r.Form.Get("xraySetting")), &next) != nil {
				t.Error("invalid template")
			}
			template = next
			saves++
			w.WriteHeader(500)
		case "/panel/api/server/status":
			result(map[string]any{"xray": map[string]any{"state": "running"}})
		case "/panel/api/server/restartXrayService":
			running = template
			reloads++
			w.WriteHeader(500)
		case "/panel/api/xray/routeTest":
			r.ParseForm()
			tag := ""
			for _, value := range running["routing"].(map[string]any)["rules"].([]any) {
				rule := value.(map[string]any)
				if users, ok := rule["user"].([]any); ok {
					match := false
					for _, v := range users {
						if v == r.Form.Get("email") {
							match = true
						}
					}
					if !match {
						continue
					}
				}
				if tags, ok := rule["inboundTag"].([]any); ok {
					found := false
					for _, v := range tags {
						if v == r.Form.Get("inboundTag") {
							found = true
						}
					}
					if !found {
						continue
					}
				}
				if port, ok := rule["port"].(string); ok && port != r.Form.Get("port") {
					continue
				}
				if patterns, ok := rule["domain"].([]any); ok {
					found := false
					for _, v := range patterns {
						pattern, _ := v.(string)
						if strings.HasPrefix(pattern, "regexp:") {
							found = found || regexp.MustCompile(strings.TrimPrefix(pattern, "regexp:")).MatchString(r.Form.Get("domain"))
						} else {
							found = found || r.Form.Get("domain") == "adservice.google.com" || r.Form.Get("domain") == "pixel.facebook.com"
						}
					}
					if !found {
						continue
					}
				}
				if network, ok := rule["network"].(string); ok && !strings.Contains(network, r.Form.Get("network")) {
					continue
				}
				tag, _ = rule["outboundTag"].(string)
				break
			}
			result(map[string]any{"matched": tag != "", "outboundTag": tag})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	_, _, panel := seedPanel(t, db, remote.URL)
	proxy, _ := sanaei.UUIDv4()
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second,generate_residential,generate_direct) VALUES('reality',true,'[443]',2,1,true,true) ON CONFLICT(policy_key) DO UPDATE SET generate_residential=true,generate_direct=true")
	sqlMust(t, db, "INSERT INTO proxies(id,name,type,host,port,status,last_success_at) VALUES($1,'residential-test','socks5','127.0.0.1',1080,'healthy',now())", proxy)
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'test','socks5','localhost',1080,'healthy',now(),'residential-ads-test')", proxy)
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,panel_ids=ARRAY[$1::uuid]", panel)
	service := residentialsync.Service{DB: db, Secrets: panelTestSecrets{}, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: panelTestSecrets{}, Timeout: time.Second}}}
	for i, want := range []map[string]int{{"DIRECT": 1, "RESIDENTIAL": 1}, {"DIRECT": 1, "RESIDENTIAL": 1}, {"DIRECT": 1, "BLOCKED": 1}} {
		if i == 1 {
			sqlMust(t, db, "UPDATE residential_proxies SET status='down' WHERE proxy_id=$1", proxy)
			sqlMust(t, db, "UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id=$1", panel)
		}
		if i == 2 {
			sqlMust(t, db, "DELETE FROM residential_proxies WHERE proxy_id=$1", proxy)
		}
		if err := service.ReconcilePanel(ctx, readyworker.Panel{ID: panel}, false); err != nil {
			t.Fatal(i, err)
		}
		var state string
		if err := db.QueryRow("SELECT state FROM panel_routing_state WHERE panel_id=$1", panel).Scan(&state); err != nil || state != "APPLIED" {
			t.Fatal(state, err)
		}
		for class, n := range want {
			var count int
			if err := db.QueryRow("SELECT count(*) FROM panel_client_routes WHERE panel_id=$1 AND effective_class=$2", panel, class).Scan(&count); err != nil || count != n {
				t.Fatal(i, class, count, err)
			}
		}
	}
	if saves != 2 || reloads != 2 {
		t.Fatal(saves, reloads)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id=$1", panel)
	if err := service.ReconcilePanel(ctx, readyworker.Panel{ID: panel}, false); err != nil {
		t.Fatal(err)
	}
	if saves != 2 || reloads != 2 {
		t.Fatal("unchanged routing restarted")
	}
}

func TestRoutingDueLanesKeepRetiredFailuresOutOfServingQueue(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	_, liveDroplet, live := seedPanel(t, db, "http://127.0.0.1:1")
	_, deadDroplet, dead := seedPanel(t, db, "http://127.0.0.1:2")
	sqlMust(t, db, "UPDATE droplets SET state='READY',expires_at=now()+interval '1 hour' WHERE id=$1", liveDroplet)
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING',expires_at=now()-interval '1 hour' WHERE id=$1", deadDroplet)
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	svc := residentialsync.Service{DB: db}
	for _, tc := range []struct {
		live bool
		id   string
	}{{true, live}, {false, dead}} {
		p, ok, err := svc.NextDuePanel(ctx, tc.live)
		if err != nil || !ok || p.ID != tc.id {
			t.Fatal(tc, p, ok, err)
		}
	}
	sqlMust(t, db, "INSERT INTO worker_item_failures(kind,item_id,next_retry_at) VALUES('residential_sync',$1,now()+interval '1 hour')", dead)
	if _, ok, err := svc.NextDuePanel(ctx, false); err != nil || ok {
		t.Fatal(ok, err)
	}
	if p, ok, err := svc.NextDuePanel(ctx, true); err != nil || !ok || p.ID != live {
		t.Fatal(p, ok, err)
	}
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,state,next_check_at) SELECT $1,revision,'APPLIED',now()+interval '20 seconds' FROM residential_routing_control", live)
	if _, ok, err := svc.NextDuePanel(ctx, true); err != nil || ok {
		t.Fatal(ok, err)
	}
	sqlMust(t, db, "UPDATE residential_routing_control SET revision=revision+1")
	if p, ok, err := svc.NextDuePanel(ctx, true); err != nil || !ok || p.ID != live {
		t.Fatal(p, ok, err)
	}
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=false")
	if _, ok, err := svc.NextDuePanel(ctx, true); err != nil || ok {
		t.Fatal(ok, err)
	}
}

func TestDashboardProviderAvailabilityIndependentOfRoutingAndPolicy(t *testing.T) {
	db := adminTestDB(t)
	account, droplet, panel := seedPanel(t, db, "http://127.0.0.1:1")
	sqlMust(t, db, "UPDATE accounts SET provider_checked_at=now() WHERE id=$1", account)
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,state) VALUES($1,'FAILED')", panel)
	sqlMust(t, db, "INSERT INTO worker_item_failures(kind,item_id,failures,last_failed_at) VALUES('residential_sync',$1,2,now())", panel)
	check := func(want string) {
		t.Helper()
		var raw []byte
		if err := db.QueryRow(dashboardCountsSQL).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"active_servers", "inactive_servers", "broken_servers", "pending_deletion_servers"} {
			expected := float64(0)
			if key == want {
				expected = 1
			}
			if m[key] != expected {
				t.Fatal(want, key, m[key])
			}
		}
	}
	check("inactive_servers")
	// Only fresh provider inventory can assert that a server is running.
	sqlMust(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) SELECT gen_random_uuid(),$1,'digitalocean','{}',jsonb_build_object('Inventory',jsonb_build_object('Servers',jsonb_build_array(jsonb_build_object('ID',provider_resource_id,'State','ready')))) FROM droplets WHERE id=$2`, account, droplet)
	check("active_servers")
	sqlMust(t, db, "UPDATE provider_snapshots SET created_at=now()-interval '5 minutes'")
	check("inactive_servers")
	sqlMust(t, db, "UPDATE worker_item_failures SET last_failed_at=now()")
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", droplet)
	check("pending_deletion_servers")
}
