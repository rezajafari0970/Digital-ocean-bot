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
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,outbound_tag) VALUES($1,'residential-ads-test')", proxy)
	if err := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev); err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET revision=$2,selected_proxy_id=$3 WHERE panel_id=$1", panel, rev, proxy)
	for i, class := range []string{"RESIDENTIAL", "DIRECT", "BLOCKED"} {
		id := fmt.Sprint(i)
		sqlMust(t, db, "INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) VALUES($1,$2,$2,'RESIDENTIAL',$3,$4)", panel, id, class, rev)
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
		req = httptest.NewRequest("GET", "/?route_class=ALL", nil)
		req.SetPathValue("token", share["token"])
		out = httptest.NewRecorder()
		s.sharedOutput(out, req)
		if out.Code != 400 {
			t.Fatal("class widened")
		}
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

func (panelTestSecrets) GetProxy(context.Context, string, string) ([]byte, error) {
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
			fmt.Fprint(w, `{"success":true,"obj":[{"id":1,"tag":"in-443-tcp","port":443,"protocol":"vless","settings":{"clients":[{"id":"a","email":"a"},{"id":"b","email":"b"}]}}]}`)
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
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,outbound_tag) VALUES($1,'residential-ads-test')", proxy)
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,panel_ids=ARRAY[$1::uuid]", panel)
	service := residentialsync.Service{DB: db, Secrets: panelTestSecrets{}, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: panelTestSecrets{}, Timeout: time.Second}}}
	for i, want := range []map[string]int{{"DIRECT": 1, "RESIDENTIAL": 1}, {"DIRECT": 1, "BLOCKED": 1}, {"DIRECT": 2}} {
		if i == 1 {
			sqlMust(t, db, "UPDATE proxies SET status='down' WHERE id=$1", proxy)
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
	if saves != 3 || reloads != 3 {
		t.Fatal(saves, reloads)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id=$1", panel)
	if err := service.ReconcilePanel(ctx, readyworker.Panel{ID: panel}, false); err != nil {
		t.Fatal(err)
	}
	if saves != 3 || reloads != 3 {
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
