package residentialperf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lib/pq"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealthSnapshotPostgresReadOnlyFreshnessAndLatestReceipt(t *testing.T) {
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated BULK_TEST_DATABASE_URL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("health_view_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
	q := u.Query()
	q.Set("search_path", schema)
	q.Set("application_name", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)
	ddl := `CREATE TABLE panel_instances(id uuid PRIMARY KEY,account_id uuid,droplet_id uuid,driver text,base_url text,auth_secret_ref text,enabled boolean);
 CREATE TABLE panel_routing_state(panel_id uuid,plan_hash text);
 CREATE TABLE residential_performance_panels(panel_id uuid,generation bigint,experiment_id uuid);
 CREATE TABLE residential_routing_control(enabled boolean,fleet boolean,panel_ids uuid[],revision bigint);
 CREATE TABLE residential_proxies(proxy_id uuid,admission_version bigint,enabled boolean);
 CREATE TABLE residential_admission_evidence(id uuid,panel_id uuid,evidence jsonb,recorded_at timestamptz DEFAULT clock_timestamp());`
	if _, err = db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	e := healthFixture(now)
	panel := e.Context.PanelID
	if _, err = db.Exec(`INSERT INTO panel_instances VALUES($1,$1,$1,'sanaei','https://example.test','ref',true);`, panel); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`INSERT INTO panel_routing_state VALUES($1,'plan')`, `INSERT INTO residential_performance_panels VALUES($1,1,$1)`, `INSERT INTO residential_routing_control VALUES(true,true,ARRAY[$1::uuid],1)`} {
		if _, err = db.Exec(query, panel); err != nil {
			t.Fatal(err)
		}
	}
	for id := range e.Context.Proxies {
		if _, err = db.Exec(`INSERT INTO residential_proxies VALUES($1,1,true)`, id); err != nil {
			t.Fatal(err)
		}
	}
	store := Store{DB: db}
	current, err := store.AdmissionContext(context.Background(), panel)
	if err != nil {
		t.Fatal(err)
	}
	e.Context = current
	save := func(e AdmissionEvidence, at time.Time) {
		t.Helper()
		raw, _ := json.Marshal(e)
		if _, err := db.Exec(`INSERT INTO residential_admission_evidence(id,panel_id,evidence,recorded_at) VALUES($1,$2,$3,$4)`, e.ID, panel, string(raw), at); err != nil {
			t.Fatal(err)
		}
	}
	view, err := store.HealthSnapshot(context.Background(), panel)
	if err != nil || view.State != "NO_EVIDENCE" {
		t.Fatal(view, err)
	}
	save(e, now)
	view, err = store.HealthSnapshot(context.Background(), panel)
	if err != nil || view.State != "CURRENT_DIAGNOSTIC" || view.MutationAllowed {
		t.Fatal(view, err)
	}
	invalid := e
	invalid.ID = "66666666-6666-4666-8666-666666666666"
	invalid.Observations = nil
	save(invalid, now.Add(time.Second))
	view, err = store.HealthSnapshot(context.Background(), panel)
	if err != nil || view.State != "INVALID_EVIDENCE" || view.EvidenceID != invalid.ID {
		t.Fatal("fell back to older green evidence", view, err)
	}
	if _, err = db.Exec(`DELETE FROM residential_admission_evidence`); err != nil {
		t.Fatal(err)
	}
	// Delay the final evidence SELECT across the freshness boundary. The report
	// must use the database timestamp after that delay, not at snapshot start.
	now = time.Now().UTC()
	e = healthFixture(now)
	e.Context = current
	e.Started = now.Add(-5*time.Minute + 250*time.Millisecond)
	for i := range e.Observations {
		e.Observations[i].Started = e.Started
	}
	save(e, now)
	lock, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	if _, err = lock.Exec(`LOCK TABLE residential_admission_evidence IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		v HealthView
		e error
	}, 1)
	go func() {
		v, e := store.HealthSnapshot(context.Background(), panel)
		result <- struct {
			v HealthView
			e error
		}{v, e}
	}()
	// Establish that the snapshot reached the blocked receipt read before release.
	deadline := time.Now().Add(3 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		var n int
		if err = db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE 'SELECT evidence FROM residential_admission_evidence%'`, schema).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			blocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("receipt query did not block")
	}
	time.Sleep(350 * time.Millisecond)
	// A concurrent context change must not tear the already-established snapshot.
	if _, err = db.Exec(`UPDATE panel_routing_state SET plan_hash='new-plan'`); err != nil {
		t.Fatal(err)
	}
	if err = lock.Commit(); err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.e != nil || got.v.State != "STALE" {
		t.Fatal("freshness used old time or torn context", got.v, got.e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = store.HealthSnapshot(ctx, panel); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM residential_admission_evidence`).Scan(&count); err != nil || count != 1 {
		t.Fatal("report mutated evidence", count, err)
	}
}
