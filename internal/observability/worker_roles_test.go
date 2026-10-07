package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	_ "github.com/lib/pq"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSplitReadinessRequiresBothFreshRoleProcesses(t *testing.T) {
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated DB required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "role_health_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	run := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	run("CREATE TABLE worker_heartbeats(worker_id text PRIMARY KEY,kind text,last_seen_at timestamptz,metadata jsonb)")
	ctx := context.Background()
	h := Health{DB: db, RequireWorker: true, WorkerMode: "split"}
	now := time.Now().Unix()
	raw, _ := json.Marshal(map[string]any{"modules": []string{"fixture"}, "supervision": map[string]any{"version": 1, "healthy": true, "modules": []map[string]any{{"name": "fixture", "state": "IDLE"}}}, "recovery_scan_unix": now, "scheduler_scan_unix": now, "lifecycle_scan_unix": now, "lifecycle_lanes": map[string]int{"in_flight": 0, "stalled": 0}, "client_mutation": map[string]any{"state": "GATED", "finished_unix": now}})
	put := func(id, kind string) {
		run("INSERT INTO worker_heartbeats VALUES($1,$2,now(),$3) ON CONFLICT(worker_id) DO UPDATE SET last_seen_at=now(),metadata=excluded.metadata", id, kind, string(raw))
	}
	ready := func(want bool) {
		t.Helper()
		r := h.Readiness(ctx)
		if (r.Status == "ready") != want {
			t.Fatalf("want ready=%v got %+v", want, r)
		}
	}
	ready(false)
	put("legacy", "production")
	ready(false)
	put("control", "production-control")
	ready(false)
	put("panels", "production-panels")
	ready(true)
	run("UPDATE worker_heartbeats SET last_seen_at=now()-interval '31 seconds' WHERE worker_id='panels'")
	ready(false)
	put("panels", "production-panels")
	ready(true)
	run("UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{recovery_scan_unix}','0') WHERE worker_id='control'")
	ready(false)
	put("control", "production-control")
	ready(true)
	run("UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{client_mutation,state}','\"FAILED\"') WHERE worker_id='panels'")
	ready(false)
	put("panels", "production-panels")
	run("UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{lifecycle_lanes,stalled}','1') WHERE worker_id='control'")
	ready(false)
	put("control", "production-control")
	run("UPDATE worker_heartbeats SET metadata='{}' WHERE worker_id='panels'")
	ready(false)
	put("panels", "production-panels")
	run("UPDATE worker_heartbeats SET last_seen_at=now()+interval '1 hour' WHERE worker_id='control'")
	ready(false)
	put("control", "production-control")
	// A fresh heartbeat cannot mask future or malformed scan timestamps.
	run("UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{scheduler_scan_unix}',to_jsonb(extract(epoch FROM now()+interval '1 hour')::bigint)) WHERE worker_id='control'")
	ready(false)
	put("control", "production-control")
	run("UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{scheduler_scan_unix}','\"bad\"') WHERE worker_id='control'")
	ready(false)
	put("control", "production-control")
	ready(true)
	run("UPDATE worker_heartbeats SET metadata=jsonb_set(metadata,'{supervision,modules,0,state}','\"STALLED\"') WHERE worker_id='control'")
	ready(false)
	put("control", "production-control")
	run("UPDATE worker_heartbeats SET metadata=metadata-'supervision' WHERE worker_id='panels'")
	ready(false)
	put("panels", "production-panels")
	h.WorkerMode = "typo"
	ready(false)
	h.WorkerMode = "all"
	ready(true)
	run("DELETE FROM worker_heartbeats WHERE worker_id='legacy'")
	ready(false)
}
