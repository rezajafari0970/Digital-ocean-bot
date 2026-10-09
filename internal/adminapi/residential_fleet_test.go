package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func fleetSeed(t *testing.T, db *sql.DB, label string) string {
	t.Helper()
	_, _, p := seedPanel(t, db, "http://"+label+".invalid")
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,plan_hash,state,pool_enabled,verified_at) SELECT $1,revision,'before','FAILED',true,NULL FROM residential_routing_control", p)
	return p
}
func fleetRequest(t *testing.T, db *sql.DB, scope string, ids []string) residentialperf.Request {
	t.Helper()
	var rev int64
	if e := db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev); e != nil {
		t.Fatal(e)
	}
	c := residentialperf.Balanced()
	return residentialperf.Request{RequestID: perfUUID(), Action: "start", Mode: "permanent", Scope: scope, Config: &c, PanelIDs: ids, BaseRevision: rev}
}
func TestPermanentFleetEnrollmentRollbackRace(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	s := residentialperf.Store{DB: db}
	one := fleetSeed(t, db, "one")
	two := fleetSeed(t, db, "offline")
	// Publication intent is allowed with a closed execution gate; no gate is reopened.
	q := fleetRequest(t, db, "fleet", nil)
	out, e := s.Do(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	var deadline sql.NullTime
	var mode, scope string
	db.QueryRow("SELECT deadline,duration_mode,publish_scope FROM residential_performance_experiments WHERE id=$1", out.ExperimentID).Scan(&deadline, &mode, &scope)
	if deadline.Valid || mode != "permanent" || scope != "fleet" || out.State != "KEPT" {
		t.Fatal("publication mode", out, mode, scope, deadline)
	}
	var enabled bool
	db.QueryRow("SELECT enabled FROM residential_routing_control").Scan(&enabled)
	if enabled {
		t.Fatal("publication reopened operator gate")
	}
	replay, e := s.Do(ctx, q)
	if e != nil || replay != out {
		t.Fatal("lost response duplicated", e)
	}
	before := map[string]any{"routing": map[string]any{"domainStrategy": "IPIfNonMatch"}}
	initial := residentialperf.Capture(before)
	for _, p := range []string{one, two} {
		if _, e = s.Prepare(ctx, p, 1, initial); e != nil {
			t.Fatal(e)
		}
		residentialperf.Apply(before, q.Config)
		if e = s.Plan(ctx, p, 1, residentialperf.Capture(before)); e != nil {
			t.Fatal(e)
		}
	}
	future := fleetSeed(t, db, "future")
	// Restarted store reads the durable global policy, not an in-memory target list.
	s = residentialperf.Store{DB: db}
	if e = s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	a, e := s.Load(ctx, future)
	if e != nil || a.Config == nil || a.Generation != 1 {
		t.Fatal("future server did not inherit", e)
	}
	late := fleetSeed(t, db, "racing")
	rollback := residentialperf.Request{RequestID: perfUUID(), Action: "rollback", ExperimentID: out.ExperimentID, ExpectedVersion: out.Version}
	var wg sync.WaitGroup
	wg.Add(2)
	var te, re error
	go func() { defer wg.Done(); te = s.Tick(ctx) }()
	go func() { defer wg.Done(); _, re = s.Do(ctx, rollback) }()
	wg.Wait()
	if te != nil || re != nil {
		t.Fatal(te, re)
	}
	after := fleetSeed(t, db, "after-rollback")
	for i := 0; i < 3; i++ {
		if e = s.Tick(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []string{one, two, future, late, after} {
		a, e = s.Load(ctx, p)
		if e != nil || a.Config != nil {
			t.Fatal("rollback left or re-enrolled a profile", p, e)
		}
	}
	a, _ = s.Load(ctx, after)
	if a.Generation != 0 {
		t.Fatal("future admission after rollback")
	}
	var state string
	db.QueryRow("SELECT state FROM residential_performance_experiments WHERE id=$1", out.ExperimentID).Scan(&state)
	if state != "ROLLING_BACK" {
		t.Fatal("offline server falsely restored", state)
	}
	var baseline []byte
	db.QueryRow("SELECT baseline FROM residential_performance_panels WHERE panel_id=$1", two).Scan(&baseline)
	if len(baseline) == 0 {
		t.Fatal("lost server baseline")
	}
	var parsed residentialperf.Fields
	json.Unmarshal(baseline, &parsed)
	if parsed.Strategy.Data != "IPIfNonMatch" {
		t.Fatal("baseline overwritten")
	}
}
func TestPermanentSelectedAndTimedFleetPromotion(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	s := residentialperf.Store{DB: db}
	one := fleetSeed(t, db, "one")
	two := fleetSeed(t, db, "two")
	q := fleetRequest(t, db, "selected", []string{one, two})
	out, e := s.Do(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	future := fleetSeed(t, db, "future")
	if e = s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	a, _ := s.Load(ctx, future)
	if a.Generation != 0 {
		t.Fatal("selected scope inherited without authorization")
	}
	// No successful runtime is invented merely because the permanent policy is saved.
	var n int
	db.QueryRow("SELECT count(*) FROM residential_performance_targets WHERE state='APPLIED'").Scan(&n)
	if n != 0 {
		t.Fatal("unverified server called applied")
	}
	pub := residentialperf.Request{RequestID: perfUUID(), Action: "publish", Scope: "fleet", ExperimentID: out.ExperimentID, ExpectedVersion: out.Version}
	out, e = s.Do(ctx, pub)
	if e != nil {
		t.Fatal(e)
	}
	a, _ = s.Load(ctx, future)
	if a.Config == nil {
		t.Fatal("existing profile promotion missed server")
	}
	again, e := s.Do(ctx, pub)
	if e != nil || again != out {
		t.Fatal("publish replay", e)
	}
	// The historical one-open-profile invariant preserves a single unambiguous rollback.
	newQ := fleetRequest(t, db, "fleet", nil)
	if _, e = s.Do(ctx, newQ); e == nil {
		t.Fatal("second profile hid rollback")
	}
}
func TestPermanentMigrationKeepsTimedBaselineAndBlocksUnsafeDowngrade(t *testing.T) {
	db := adminTestDBThrough(t, 151)
	ctx := context.Background()
	_, _, panel := seedPanel(t, db, "http://migration.invalid")
	id := perfUUID()
	c, _ := json.Marshal(residentialperf.Balanced())
	sqlMust(t, db, "INSERT INTO residential_performance_experiments(id,spec,state,deadline) VALUES($1,$2,'RUNNING',now()+interval '15 minutes')", id, string(c))
	sqlMust(t, db, "INSERT INTO residential_performance_panels(panel_id,experiment_id,config,baseline) VALUES($1,$2,$3,'{\"routing_strategy\":{\"present\":true,\"value\":\"AsIs\"}}')", panel, id, string(c))
	sqlMust(t, db, "INSERT INTO residential_performance_targets(experiment_id,panel_id,generation) VALUES($1,$2,1)", id, panel)
	up, e := os.ReadFile(filepath.Join("..", "..", "migrations", "000152_residential_permanent_publish.up.sql"))
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, string(up))
	var mode string
	var baseline []byte
	db.QueryRow("SELECT duration_mode FROM residential_performance_experiments WHERE id=$1", id).Scan(&mode)
	db.QueryRow("SELECT baseline FROM residential_performance_panels WHERE panel_id=$1", panel).Scan(&baseline)
	if mode != "timed" || len(baseline) == 0 {
		t.Fatal("migration changed active trial")
	}
	tuningUp, e := os.ReadFile(filepath.Join("..", "..", "migrations", "000165_residential_tuning.up.sql"))
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, string(tuningUp))
	s := residentialperf.Store{DB: db}
	_, e = s.Do(ctx, residentialperf.Request{RequestID: perfUUID(), ExperimentID: id, ExpectedVersion: 1, Action: "publish", Scope: "fleet"})
	if e != nil {
		t.Fatal(e)
	}
	down, e := os.ReadFile(filepath.Join("..", "..", "migrations", "000152_residential_permanent_publish.down.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("active global policy lost through downgrade")
	}
}
