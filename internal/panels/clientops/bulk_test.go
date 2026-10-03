package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestBulkObservedRecoveryAndConflicts(t *testing.T) {
	c := sanaei.Client{ID: "u1", Email: "planned", Enable: true, TotalGB: 100, LimitHWID: 2}
	snapshot := func(clients []sanaei.Client) []json.RawMessage {
		b, _ := json.Marshal(map[string]any{"id": 1, "enable": true, "settings": map[string]any{"clients": clients}})
		return []json.RawMessage{b}
	}
	got, missing, err := bulkObserved(snapshot(nil), 1, []sanaei.Client{c})
	if err != nil || len(got) != 0 || len(missing) != 1 {
		t.Fatal(got, missing, err)
	}
	runtime := c
	runtime.LimitHWID = 0
	got, missing, err = bulkObserved(snapshot([]sanaei.Client{runtime}), 1, []sanaei.Client{c})
	if err != nil || len(got) != 1 || len(missing) != 0 {
		t.Fatal(got, missing, err)
	}
	runtime.ID = "someone-else"
	if _, _, err = bulkObserved(snapshot([]sanaei.Client{runtime}), 1, []sanaei.Client{c}); !errors.Is(err, ErrClientConflict) {
		t.Fatal(err)
	}
	if _, _, err = bulkObserved([]json.RawMessage{json.RawMessage(`{"id":1,"enable":true,"settings":"truncated"}`)}, 1, []sanaei.Client{c}); err == nil {
		t.Fatal("malformed snapshot must not mean absent")
	}
}

func TestBulkRejectsDuplicateIdentityAndEmail(t *testing.T) {
	p := BulkPayload{GenerationID: "g", TargetUsers: 10, Clients: []sanaei.Client{{ID: "a", Email: "x", Enable: true}, {ID: "b", Email: "x", Enable: true}}}
	if p.Validate() == nil {
		t.Fatal("duplicate email")
	}
	p.Clients[1].Email = "y"
	p.Clients[1].ID = "a"
	if p.Validate() == nil {
		t.Fatal("duplicate UUID")
	}
}

func bulkTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("test database must contain bulk_test")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sanaei.UUIDv4()
	schema := "bulk_" + strings.ReplaceAll(id, "-", "")
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { db.Close(); admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); admin.Close() })
	fixture := `CREATE TABLE accounts(id uuid PRIMARY KEY,enabled boolean,provider_state text);
 CREATE TABLE droplets(id uuid PRIMARY KEY,state text);
 CREATE TABLE deployments(droplet_id uuid,state text);
 CREATE TABLE panel_instances(id uuid PRIMARY KEY,account_id uuid,droplet_id uuid,enabled boolean);
 CREATE TABLE panel_inbound_inventory(panel_id uuid,remote_id bigint,present boolean,enabled boolean);`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000124_client_mutation_jobs", "000125_bulk_user_ownership", "000127_client_mutation_execution_gate", "000131_bulk_user_unknown_outcome_recovery", "000134_durable_bulk_create", "000135_bulk_scale_recovery"} {
		b, e := os.ReadFile(filepath.Join("../../../migrations", name+".up.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(name, e)
		}
	}
	_, err = db.Exec(`INSERT INTO accounts VALUES('11111111-1111-4111-8111-111111111111',true,'ACTIVE');
 INSERT INTO droplets VALUES('22222222-2222-4222-8222-222222222222','READY');
 INSERT INTO deployments VALUES('22222222-2222-4222-8222-222222222222','PANEL_COMPLETE');
 INSERT INTO panel_instances VALUES('33333333-3333-4333-8333-333333333333','11111111-1111-4111-8111-111111111111','22222222-2222-4222-8222-222222222222',true);
 INSERT INTO panel_inbound_inventory VALUES('33333333-3333-4333-8333-333333333333',1,true,true);
 INSERT INTO bulk_user_generations(id,panel_id,inbound_id,purpose,marker) VALUES('44444444-4444-4444-8444-444444444444','33333333-3333-4333-8333-333333333333',1,'CANARY','test');
 UPDATE bulk_client_execution_gate SET enabled=true,kill_switch=false,panel_id='33333333-3333-4333-8333-333333333333',inbound_id=1,remaining_batches=1,expires_at=now()+interval '5 minutes';`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func bulkTestPayload(n int) BulkPayload {
	p := BulkPayload{GenerationID: "44444444-4444-4444-8444-444444444444", TargetUsers: n}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%08d-0000-4000-8000-000000000001", i+1)
		p.Clients = append(p.Clients, sanaei.Client{ID: id, Email: fmt.Sprintf("u-test-%08d", i+1), Enable: true, TotalGB: 100, LimitHWID: 2, Flow: "xtls-rprx-vision"})
	}
	return p
}

func TestBulkPostgresAtomicPlanRaceAndGate(t *testing.T) {
	db := bulkTestDB(t)
	j := Journal{DB: db}
	ctx := context.Background()
	p := bulkTestPayload(10)
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := j.ReserveBulk(ctx, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1, p)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var jobs, owned int
	db.QueryRow(`SELECT count(*) FROM client_mutation_jobs`).Scan(&jobs)
	db.QueryRow(`SELECT count(*) FROM bulk_user_ownership WHERE state='PLANNED'`).Scan(&owned)
	if jobs != 1 || owned != 10 {
		t.Fatalf("jobs=%d planned=%d", jobs, owned)
	}
	if _, ok, err := j.Claim(ctx); err != nil || ok {
		t.Fatal("closed main gate", ok, err)
	}
	db.Exec(`UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false`)
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok || job.Attempts != 1 {
		t.Fatal(job, ok, err)
	}
	if _, ok, err = j.Claim(ctx); err != nil || ok {
		t.Fatal("finite gate", ok, err)
	}
	unlock, ok, err := j.executorLock(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if unlock2, ok2, e := j.executorLock(ctx); e != nil || ok2 {
		if ok2 {
			unlock2()
		}
		t.Fatal("cross-process executor lock", ok2, e)
	}
	unlock()
}

func TestBulkPostgresLostResponsePartialRetryOnlyMissing(t *testing.T) {
	db := bulkTestDB(t)
	j := Journal{DB: db}
	ctx := context.Background()
	p := bulkTestPayload(10)
	id, ok, err := j.ReserveBulk(ctx, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1, p)
	if err != nil || !ok {
		t.Fatal(err)
	}
	db.Exec(`UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false`)
	job, ok, err := j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	var mu sync.Mutex
	state := map[string]sanaei.Client{}
	batches := [][]sanaei.Client{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "inbounds/list"):
			cs := []sanaei.Client{}
			for _, c := range state {
				c.LimitHWID = 0
				cs = append(cs, c)
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": []any{map[string]any{"id": 1, "enable": true, "settings": map[string]any{"clients": cs}}}})
		case strings.HasSuffix(r.URL.Path, "clients/list"):
			records := []sanaei.GlobalClient{}
			for _, c := range state {
				records = append(records, sanaei.GlobalClient{UUID: c.ID, Email: c.Email, Enable: c.Enable, TotalGB: c.TotalGB, ExpiryTime: c.ExpiryTime, LimitHWID: c.LimitHWID, InboundIDs: []int64{1}})
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": records})
		case strings.Contains(r.URL.Path, "clients/get/"):
			c, found := state[strings.TrimPrefix(r.URL.Path, "/panel/api/clients/get/")]
			if !found {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": map[string]any{"client": map[string]any{"uuid": c.ID, "email": c.Email, "enable": c.Enable, "totalGB": c.TotalGB, "expiryTime": c.ExpiryTime, "limitHwid": c.LimitHWID}}})
		case strings.HasSuffix(r.URL.Path, "clients/bulkCreate"):
			var items []struct {
				Client sanaei.Client `json:"client"`
			}
			json.NewDecoder(r.Body).Decode(&items)
			batch := []sanaei.Client{}
			for _, item := range items {
				batch = append(batch, item.Client)
			}
			batches = append(batches, batch)
			limit := len(batch)
			if len(batches) == 1 {
				limit = 3
			}
			for _, c := range batch[:limit] {
				state[c.Email] = c
			}
			if len(batches) == 1 {
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": map[string]any{"created": len(batch), "skipped": []any{}}})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := sanaei.NewAPIClient(server.URL, sanaei.Credentials{Username: "test", Password: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := &sanaei.PanelRuntime{Session: sanaei.NewPanelSession(client)}
	e := Executor{Journal: j}
	if err = e.executeBulk(ctx, rt, job); err == nil {
		t.Fatal("partial commit must fail-close")
	}
	var active, planned int
	db.QueryRow(`SELECT count(*) FILTER(WHERE state='ACTIVE'),count(*) FILTER(WHERE state='PLANNED') FROM bulk_user_ownership`).Scan(&active, &planned)
	if active != 3 || planned != 7 {
		t.Fatalf("active=%d planned=%d", active, planned)
	}
	if err = j.Retry(ctx, id, "lost response", 0); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE bulk_client_execution_gate SET remaining_batches=1`)
	job, ok, err = j.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = e.executeBulk(ctx, rt, job); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(batches) != 2 || len(batches[0]) != 10 || len(batches[1]) != 7 {
		t.Fatal("wrong retry subset", batches)
	}
	seen := map[string]bool{}
	for _, c := range batches[0][:3] {
		seen[c.ID] = true
	}
	for _, c := range batches[1] {
		if seen[c.ID] {
			t.Fatal("duplicate create", c.ID)
		}
	}
	mu.Unlock()
	db.QueryRow(`SELECT count(*) FROM bulk_user_ownership WHERE state='ACTIVE'`).Scan(&active)
	if active != 10 {
		t.Fatal(active)
	}
	if err = j.Succeed(ctx, id); err != nil {
		t.Fatal(err)
	}
	// A recovered committed batch performs no additional POST.
	db.Exec(`UPDATE client_mutation_jobs SET state='RUNNING' WHERE id=$1`, id)
	if err = e.executeBulk(context.Background(), rt, job); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatal("committed retry posted again")
	}
}
