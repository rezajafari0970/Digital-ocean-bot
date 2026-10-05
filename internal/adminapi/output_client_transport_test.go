package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func transportFixture(t *testing.T, db *sql.DB, host string) string {
	t.Helper()
	_, _, panel := seedPanel(t, db, "http://"+host+":2053")
	sqlMust(t, db, `UPDATE residential_routing_control SET enabled=true,fleet=true`)
	sqlMust(t, db, `INSERT INTO panel_routing_state(panel_id,revision,state,pool_enabled,verified_at) SELECT $1,revision,'APPLIED',true,now() FROM residential_routing_control`, panel)
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		id, _ := sanaei.UUIDv4()
		uri := "vless://" + id + "@" + host + ":443?security=reality&type=tcp&flow=xtls-rprx-vision-udp443&encryption=none&sni=example.com&fp=chrome&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=0123#" + class
		sqlMust(t, db, `INSERT INTO panel_client_routes(panel_id,client_id,email,route_class,effective_class,revision) SELECT $1,$2,$2,$3,$3,revision FROM residential_routing_control`, panel, id, class)
		sqlMust(t, db, `INSERT INTO output_config_snapshots(panel_id,uri,client_id) VALUES($1,$2,$3)`, panel, uri, id)
	}
	return panel
}
func transportPut(s *Server, panel, preset string, rev int64, op string, role auth.Role) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"preset":%q,"expected_revision":%d,"operation_id":%q}`, preset, rev, op)
	r := httptest.NewRequest("PUT", "/", strings.NewReader(body))
	r.SetPathValue("id", panel)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Username: "test-admin", Role: role}))
	w := httptest.NewRecorder()
	s.putOutputClientTransport(w, r)
	return w
}
func TestClientTransportAtomicScopeRollbackAndSharing(t *testing.T) {
	db := adminTestDB(t)
	panel := transportFixture(t, db, "203.0.113.1")
	transportFixture(t, db, "203.0.113.2")
	proxy, _ := sanaei.UUIDv4()
	sqlMust(t, db, `INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'fixture','socks5','localhost',1080,'healthy',now(),'residential-ads-test')`, proxy)
	// Proxy insertion advances the routing revision; refresh fixture proof.
	sqlMust(t, db, `UPDATE panel_routing_state SET revision=(SELECT revision FROM residential_routing_control); UPDATE panel_client_routes SET revision=(SELECT revision FROM residential_routing_control)`)
	s := &Server{DB: db}
	op, _ := sanaei.UUIDv4()
	var before time.Time
	db.QueryRow(`SELECT min(first_seen_at) FROM output_config_snapshots WHERE panel_id=$1`, panel).Scan(&before)
	w := transportPut(s, panel, "layered-balanced-v1", 0, op, auth.Admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = transportPut(s, panel, "layered-balanced-v1", 0, op, auth.Admin); w.Code != 200 || !strings.Contains(w.Body.String(), "replayed") {
		t.Fatal("lost response replay", w.Code)
	}
	if w = transportPut(s, panel, "off", 0, op, auth.Admin); w.Code != 409 {
		t.Fatal("reused operation accepted", w.Code)
	}
	future := transportFixture(t, db, "203.0.113.3")
	_ = future
	sqlMust(t, db, `UPDATE panel_routing_state SET revision=(SELECT revision FROM residential_routing_control); UPDATE panel_client_routes SET revision=(SELECT revision FROM residential_routing_control)`)
	for _, class := range []string{"DIRECT", "RESIDENTIAL", "ALL"} {
		w = httptest.NewRecorder()
		s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), class)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		count := 0
		for _, line := range strings.Fields(w.Body.String()) {
			u, e := url.Parse(line)
			if e != nil {
				t.Fatal(e)
			}
			if u.Hostname() == "203.0.113.1" {
				if !u.Query().Has("fm") {
					t.Fatal("target mask missing")
				}
				count++
			} else if u.Query().Has("fm") {
				t.Fatal("mask leaked to another/future server")
			}
			if class != "ALL" && u.Fragment != class {
				t.Fatal("class leaked")
			}
		}
		want := 1
		if class == "ALL" {
			want = 2
		}
		if count != want {
			t.Fatalf("class %s target count %d", class, count)
		}
		token := newShareToken()
		sqlMust(t, db, `INSERT INTO output_share_tokens(token,route_class) VALUES($1,$2)`, token, class)
		req := httptest.NewRequest("GET", "/?format=xray-json", nil)
		req.SetPathValue("token", token)
		w = httptest.NewRecorder()
		s.sharedOutput(w, req)
		var configs []map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &configs) != nil || len(configs) < 3 {
			t.Fatal("JSON share", w.Code, w.Body.String())
		}
		req = httptest.NewRequest("GET", "/?route_class=BLOCKED", nil)
		req.SetPathValue("token", token)
		w = httptest.NewRecorder()
		s.sharedOutput(w, req)
		if w.Code != 400 {
			t.Fatal("token class override")
		}
	}
	// A malformed late snapshot on one target must not break the subscription.
	sqlMust(t, db, `INSERT INTO output_config_snapshots(panel_id,uri,client_id) SELECT panel_id,'invalid-uri',client_id FROM output_config_snapshots WHERE panel_id=$1 LIMIT 1`, panel)
	w = httptest.NewRecorder()
	s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "ALL")
	if w.Code != 200 || w.Header().Get("X-Output-Omitted-Invalid") != "1" || strings.Contains(w.Body.String(), "invalid-uri") {
		t.Fatal("fault isolation", w.Code, w.Body.String())
	}
	sqlMust(t, db, `DELETE FROM output_config_snapshots WHERE uri='invalid-uri'`)
	var after time.Time
	db.QueryRow(`SELECT min(first_seen_at) FROM output_config_snapshots WHERE panel_id=$1`, panel).Scan(&after)
	if !after.Equal(before) {
		t.Fatal("publication reset first-seen")
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM output_config_snapshots WHERE uri LIKE '%fm=%'`).Scan(&n)
	if n != 0 {
		t.Fatal("snapshot mutated")
	}
	// Server retirement hides its links but does not prevent rollback.
	sqlMust(t, db, `UPDATE droplets SET state='DELETED' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)`, panel)
	w = httptest.NewRecorder()
	s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "ALL")
	if strings.Contains(w.Body.String(), "203.0.113.1") {
		t.Fatal("retired server published")
	}
	next, _ := sanaei.UUIDv4()
	w = transportPut(s, panel, "off", 1, next, auth.Admin)
	if w.Code != 200 {
		t.Fatal("retired rollback", w.Code, w.Body.String())
	}
	sqlMust(t, db, `UPDATE droplets SET state='READY' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)`, panel)
	w = httptest.NewRecorder()
	s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "ALL")
	if strings.Contains(w.Body.String(), "fm=") {
		t.Fatal("rollback not reflected")
	}
	sqlMust(t, db, `UPDATE output_config_snapshots SET last_seen_at=now()-interval '16 seconds' WHERE panel_id=$1`, panel)
	w = httptest.NewRecorder()
	s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), "ALL")
	if strings.Contains(w.Body.String(), "203.0.113.1") {
		t.Fatal("stale output escaped filter")
	}
	next, _ = sanaei.UUIDv4()
	if w = transportPut(s, panel, "tcp-balanced-v1", 2, next, auth.Admin); w.Code != 409 {
		t.Fatal("stale server activation", w.Code)
	}
}
func TestClientTransportConcurrentRevisionAndAuthorization(t *testing.T) {
	db := adminTestDB(t)
	panel := transportFixture(t, db, "203.0.113.4")
	s := &Server{DB: db}
	for _, role := range []auth.Role{auth.Operator, auth.Viewer} {
		op, _ := sanaei.UUIDv4()
		if w := transportPut(s, panel, "tcp-balanced-v1", 0, op, role); w.Code != 403 {
			t.Fatal("non-admin mutation")
		}
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op, _ := sanaei.UUIDv4()
			codes <- transportPut(s, panel, "tcp-balanced-v1", 0, op, auth.Admin).Code
		}()
	}
	wg.Wait()
	close(codes)
	n := map[int]int{}
	for c := range codes {
		n[c]++
	}
	if n[200] != 1 || n[409] != 1 {
		t.Fatal(n)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='output_client_transport_changed'`).Scan(&count)
	if count != 1 {
		t.Fatal("audit not atomic", count)
	}
}
