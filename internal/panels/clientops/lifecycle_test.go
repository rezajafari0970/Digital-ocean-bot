package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const lifePanel = "33333333-3333-4333-8333-333333333333"
const lifeGeneration = "44444444-4444-4444-8444-444444444444"

type lifeServer struct {
	mu                                       sync.Mutex
	clients                                  map[string]sanaei.Client
	used                                     map[string]int64
	postCreates, postUpdates, postDeletes    int
	deleteSizes                              []int
	loseUpdate, partialDelete, partialCreate bool
	createSizes                              []int
}

func (s *lifeServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj := func(v any) { json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": v}) }
	switch {
	case strings.HasSuffix(r.URL.Path, "inbounds/list"):
		cs := []sanaei.Client{}
		for _, c := range s.clients {
			c.LimitHWID = 0
			cs = append(cs, c)
		}
		obj([]any{map[string]any{"id": 1, "port": 443, "enable": true, "protocol": "vless", "streamSettings": map[string]any{"network": "tcp", "security": "reality"}, "settings": map[string]any{"clients": cs}}})
	case strings.HasSuffix(r.URL.Path, "clients/list"):
		cs := []sanaei.GlobalClient{}
		for _, c := range s.clients {
			cs = append(cs, sanaei.GlobalClient{UUID: c.ID, Email: c.Email, Enable: c.Enable, TotalGB: c.TotalGB, ExpiryTime: c.ExpiryTime, LimitHWID: c.LimitHWID, InboundIDs: []int64{1}, Traffic: &sanaei.ClientTraffic{Email: c.Email, Up: s.used[c.Email]}})
		}
		obj(cs)
	case strings.Contains(r.URL.Path, "clients/get/"):
		c, ok := s.clients[strings.TrimPrefix(r.URL.Path, "/panel/api/clients/get/")]
		if !ok {
			w.WriteHeader(404)
			return
		}
		obj(map[string]any{"client": map[string]any{"uuid": c.ID, "email": c.Email, "enable": c.Enable, "totalGB": c.TotalGB, "expiryTime": c.ExpiryTime, "limitHwid": c.LimitHWID, "flow": c.Flow, "tgId": 456, "subId": "keep-me"}})
	case strings.HasSuffix(r.URL.Path, "clients/bulkCreate"):
		var items []struct {
			Client sanaei.Client `json:"client"`
		}
		if json.NewDecoder(r.Body).Decode(&items) != nil {
			w.WriteHeader(400)
			return
		}
		s.postCreates++
		s.createSizes = append(s.createSizes, len(items))
		originalN := len(items)
		if s.partialCreate {
			items = items[:1]
		}
		for _, it := range items {
			if _, ok := s.clients[it.Client.Email]; ok {
				w.WriteHeader(409)
				return
			}
			s.clients[it.Client.Email] = it.Client
		}
		if s.partialCreate {
			s.partialCreate = false
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		obj(map[string]any{"created": originalN, "skipped": []any{}})
	case strings.Contains(r.URL.Path, "clients/update/"):
		var m map[string]any
		if json.NewDecoder(r.Body).Decode(&m) != nil {
			w.WriteHeader(400)
			return
		}
		email := strings.TrimPrefix(r.URL.Path, "/panel/api/clients/update/")
		c, ok := s.clients[email]
		if !ok || email == "manual" || m["subId"] != "keep-me" || m["tgId"] != float64(456) {
			w.WriteHeader(400)
			return
		}
		b, _ := json.Marshal(m)
		var updated sanaei.Client
		_ = json.Unmarshal(b, &updated)
		if updated.ID != c.ID {
			w.WriteHeader(400)
			return
		}
		s.clients[email] = updated
		s.postUpdates++
		if s.loseUpdate {
			s.loseUpdate = false
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		obj(nil)
	case strings.HasSuffix(r.URL.Path, "clients/bulkDel"):
		var p struct {
			Emails []string `json:"emails"`
		}
		_ = json.NewDecoder(r.Body).Decode(&p)
		s.postDeletes++
		s.deleteSizes = append(s.deleteSizes, len(p.Emails))
		n := len(p.Emails)
		if s.partialDelete {
			n = 1
		}
		for _, email := range p.Emails[:n] {
			if email == "manual" {
				panic("manual delete")
			}
			delete(s.clients, email)
		}
		if s.partialDelete {
			s.partialDelete = false
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		obj(map[string]any{"deleted": n, "skipped": []any{}})
	default:
		w.WriteHeader(404)
	}
}
func lifecycleFixture(t *testing.T) (*sql.DB, Journal, *sanaei.PanelRuntime, *lifeServer) {
	t.Helper()
	db := bulkTestDB(t)
	_, err := db.Exec(`CREATE TABLE global_config_policies(policy_key text PRIMARY KEY,enabled boolean,ports jsonb,target_users_per_inbound int,user_quota_bytes bigint,user_lifetime_seconds int,device_limit int,users_per_second int);
 INSERT INTO global_config_policies VALUES('reality',false,'[443]',1,0,0,0,1);
 CREATE TABLE user_capacity_snapshots(panel_id uuid,inbound_id bigint,port int,target_users int,active_users int,deficit int,last_error text,observed_at timestamptz,PRIMARY KEY(panel_id,inbound_id));
 UPDATE bulk_lifecycle_control SET enabled=true;
 UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false;
 UPDATE bulk_client_execution_gate SET enabled=false,kill_switch=true;
 INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,target_users,quota_bytes,lifetime_seconds,device_limit,users_per_second,max_batch_size,remaining_operations,expires_at) VALUES('33333333-3333-4333-8333-333333333333',1,'44444444-4444-4444-8444-444444444444',true,false,true,4,100,600,2,100,10,30,now()+interval '1 hour');`)
	if err != nil {
		t.Fatal(err)
	}
	state := &lifeServer{clients: map[string]sanaei.Client{"manual": {ID: "manual-id", Email: "manual", Enable: true, Flow: "xtls-rprx-vision"}}, used: map[string]int64{}}
	server := httptest.NewServer(http.HandlerFunc(state.serve))
	t.Cleanup(server.Close)
	c, err := sanaei.NewAPIClient(server.URL, sanaei.Credentials{Username: "test", Password: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := &sanaei.PanelRuntime{PanelID: lifePanel, AccountID: "11111111-1111-4111-8111-111111111111", Session: sanaei.NewPanelSession(c)}
	return db, Journal{DB: db}, rt, state
}
func planLife(t *testing.T, j Journal, rt *sanaei.PanelRuntime) bool {
	t.Helper()
	ok, err := j.PlanLifecycle(context.Background(), rt, 1, func(_ context.Context, rate, n int) (int, error) { return n, nil })
	if err != nil {
		t.Fatal(err)
	}
	return ok
}
func runLife(t *testing.T, j Journal, rt *sanaei.PanelRuntime) Job {
	t.Helper()
	job, ok, err := j.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); err != nil {
		t.Fatal(err)
	}
	if err = j.Succeed(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestLifecyclePlanPolicyQuotaRecoveryReplacementAndCleanup(t *testing.T) {
	db, j, rt, state := lifecycleFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := j.PlanLifecycle(ctx, rt, 1, func(_ context.Context, rate, n int) (int, error) { return n, nil })
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var jobs, planned int
	db.QueryRow(`SELECT count(*) FROM client_mutation_jobs`).Scan(&jobs)
	db.QueryRow(`SELECT count(*) FROM bulk_user_ownership WHERE state='PLANNED'`).Scan(&planned)
	if jobs != 1 || planned != 3 || state.postCreates != 0 {
		t.Fatal(jobs, planned, state.postCreates)
	}
	first := runLife(t, j, rt)
	if first.Kind != KindBulkCreate {
		t.Fatal(first.Kind)
	}
	_, err := db.Exec(`UPDATE bulk_lifecycle_scopes SET quota_bytes=200,lifetime_seconds=300,device_limit=3`)
	if err != nil {
		t.Fatal(err)
	}
	state.loseUpdate = true
	for n := 0; n < 3; n++ {
		if !planLife(t, j, rt) {
			t.Fatal("missing policy job")
		}
		job := runLife(t, j, rt)
		if job.Kind != KindUpdate {
			t.Fatal(job.Kind)
		}
	}
	if state.postUpdates != 3 {
		t.Fatal("lost update response duplicated POST", state.postUpdates)
	}
	if planLife(t, j, rt) {
		t.Fatal("stable policy produced a job")
	}
	state.mu.Lock()
	for email, c := range state.clients {
		if email == "manual" {
			continue
		}
		if c.TotalGB != 200 || c.LimitHWID != 3 {
			t.Fatal(c)
		}
		state.used[email] = 200
	}
	state.partialDelete = true
	state.mu.Unlock()
	if !planLife(t, j, rt) {
		t.Fatal("quota cleanup missing")
	}
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok || job.Kind != KindBulkDelete {
		t.Fatal(ok, err, job.Kind)
	}
	if err = (Executor{Journal: j}).executeRuntime(ctx, rt, job); err == nil {
		t.Fatal("partial deletion reported success")
	}
	if err = j.Retry(ctx, job.ID, "lost response", 0); err != nil {
		t.Fatal(err)
	}
	if err = j.FailCloseGate(ctx); err != nil {
		t.Fatal(err)
	}
	if planLife(t, j, rt) {
		t.Fatal("replacement while outcome unresolved")
	}
	current, _ := j.Get(ctx, job.ID)
	present, err := (Executor{Journal: j}).ReconcileBulkDeleteOnly(ctx, rt, current)
	if err != nil || len(present) != 2 {
		t.Fatal(present, err)
	}
	db.Exec(`UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false`)
	recovered := runLife(t, j, rt)
	if recovered.ID != job.ID || recovered.Attempts != 2 {
		t.Fatal(recovered)
	}
	if len(state.deleteSizes) != 2 || state.deleteSizes[0] != 3 || state.deleteSizes[1] != 2 {
		t.Fatal(state.deleteSizes)
	}
	if !planLife(t, j, rt) {
		t.Fatal("replacement missing")
	}
	replacement := runLife(t, j, rt)
	if replacement.Kind != KindBulkCreate || replacement.ID == first.ID {
		t.Fatal(replacement)
	}
	db.Exec(`UPDATE bulk_lifecycle_scopes SET allow_create=false,target_users=0`)
	if !planLife(t, j, rt) {
		t.Fatal("final cleanup missing")
	}
	runLife(t, j, rt)
	if planLife(t, j, rt) {
		t.Fatal("manual cleanup or disabled replacement planned")
	}
	if len(state.clients) != 1 || state.clients["manual"].ID != "manual-id" {
		t.Fatal("baseline changed")
	}
	var active, deleted int
	db.QueryRow(`SELECT count(*) FILTER(WHERE state='ACTIVE'),count(*) FILTER(WHERE state='DELETED') FROM bulk_user_ownership`).Scan(&active, &deleted)
	if active != 0 || deleted != 6 {
		t.Fatal(active, deleted)
	}
}
func TestLifecycleFreshDeleteGuardRenewalAndBudget(t *testing.T) {
	db, j, rt, state := lifecycleFixture(t)
	ctx := context.Background()
	if !planLife(t, j, rt) {
		t.Fatal("plan")
	}
	runLife(t, j, rt)
	state.mu.Lock()
	for email, c := range state.clients {
		if email != "manual" {
			c.ExpiryTime = time.Now().Add(-time.Second).UnixMilli()
			state.clients[email] = c
		}
	}
	state.mu.Unlock()
	if !planLife(t, j, rt) {
		t.Fatal("expired plan missing")
	}
	var id string
	db.QueryRow(`SELECT id::text FROM client_mutation_jobs WHERE state='PENDING'`).Scan(&id)
	if _, err := db.Exec(`UPDATE client_mutation_jobs SET payload='{}' WHERE id=$1`, id); err == nil {
		t.Fatal("immutable plan bypass")
	}
	if _, ok, err := j.ClaimID(ctx, id); err != nil || ok {
		t.Fatal("manual executor bypass", ok, err)
	}
	db.Exec(`UPDATE bulk_lifecycle_scopes SET remaining_operations=0`)
	if _, ok, err := j.Claim(ctx); err != nil || ok {
		t.Fatal("budget bypass", ok, err)
	}
	db.Exec(`UPDATE bulk_lifecycle_scopes SET remaining_operations=1`)
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	state.mu.Lock()
	for email, c := range state.clients {
		if email != "manual" {
			c.ExpiryTime = time.Now().Add(time.Hour).UnixMilli()
			state.clients[email] = c
		}
	}
	state.mu.Unlock()
	if err = (Executor{Journal: j}).executeRuntime(ctx, rt, job); !errors.Is(err, ErrClientConflict) {
		t.Fatal("renewed client deletion", err)
	}
	if state.postDeletes != 0 {
		t.Fatal("stale delete posted")
	}
}
func TestLifecycleTrafficMissingAndOverflowFailClosed(t *testing.T) {
	c := LifecycleClient{Client: sanaei.Client{Enable: true, Email: "a", TotalGB: 100}}
	if _, err := c.InactiveReason(time.Now()); err == nil {
		t.Fatal("missing stats accepted")
	}
	c.Traffic = &sanaei.ClientTraffic{Email: "a", Up: math.MaxInt64, Down: math.MaxInt64}
	if reason, err := c.InactiveReason(time.Now()); err != nil || reason != "QUOTA" {
		t.Fatal(reason, err)
	}
	c.Traffic.Email = "other"
	if _, err := c.InactiveReason(time.Now()); err == nil {
		t.Fatal("wrong stats identity")
	}
}

func TestLifecycleScopeEnumerationAndSupersededCreate(t *testing.T) {
	db, j, rt, state := lifecycleFixture(t)
	ctx := context.Background()
	ids, err := j.LifecycleInbounds(ctx, lifePanel)
	if err != nil || len(ids) != 1 || ids[0] != 1 {
		t.Fatal(ids, err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("plan missing")
	}
	db.Exec(`UPDATE bulk_lifecycle_scopes SET allow_create=false`)
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err = (Executor{Journal: j}).executeRuntime(ctx, rt, job); !errors.Is(err, ErrLifecycleSuperseded) {
		t.Fatal(err)
	}
	if state.postCreates != 0 {
		t.Fatal("disabled policy posted")
	}
	if err = j.supersedeLifecycle(ctx, job); err != nil {
		t.Fatal(err)
	}
	var aborted int
	db.QueryRow(`SELECT count(*) FROM bulk_user_ownership WHERE state='ABORTED'`).Scan(&aborted)
	if aborted != 3 {
		t.Fatal(aborted)
	}
	if planLife(t, j, rt) {
		t.Fatal("disabled creation continued")
	}
	db.Exec(`UPDATE bulk_lifecycle_scopes SET allow_create=true`)
	if !planLife(t, j, rt) {
		t.Fatal("new authorized plan missing")
	}
	runLife(t, j, rt)
}

func TestLifecycleBlocksOrdinaryMutationAndScopeExpiry(t *testing.T) {
	db, j, rt, _ := lifecycleFixture(t)
	ctx := context.Background()
	job, _, err := j.Reserve(ctx, Request{AccountID: rt.AccountID, PanelID: rt.PanelID, InboundID: 1, ClientID: "manual-id", Kind: KindUpdate, IdempotencyKey: "ordinary", Payload: json.RawMessage(`{"Patch":{"limitHwid":2}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if planLife(t, j, rt) {
		t.Fatal("ordinary mutation race")
	}
	db.Exec(`UPDATE client_mutation_jobs SET state='SUCCEEDED' WHERE id=$1`, job.ID)
	if !planLife(t, j, rt) {
		t.Fatal("authorized plan missing")
	}
	db.Exec(`UPDATE bulk_lifecycle_scopes SET expires_at=now()-interval '1 second'`)
	if _, ok, err := j.Claim(ctx); err != nil || ok {
		t.Fatal("expired scope claim", ok, err)
	}
}
