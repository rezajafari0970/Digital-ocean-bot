package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type tuningFixture struct {
	t                 *testing.T
	db                *sql.DB
	s                 residentialperf.Store
	id                string
	panels            []string
	before, candidate residentialperf.Config
	rev               int64
}

func newTuningFixture(t *testing.T, n int) *tuningFixture {
	t.Helper()
	f := &tuningFixture{t: t, db: adminTestDB(t)}
	f.s = residentialperf.Store{DB: f.db}
	sqlMust(t, f.db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	proxy := perfUUID()
	sqlMust(t, f.db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'test','socks5','localhost',1080,'healthy',now(),'residential-ads-test')", proxy)
	for i := 0; i < n; i++ {
		f.panels = append(f.panels, fleetSeed(t, f.db, fmt.Sprintf("tune-%d", i)))
	}
	f.before = residentialperf.Balanced()
	f.before.FastCount = 13
	f.before.FastOpen = true
	f.before.Costs = map[string]float64{proxy: 1.25}
	f.candidate = f.before
	f.candidate.FastCount = 3
	f.candidate.FastShare = 90
	q := fleetRequest(t, f.db, "fleet", nil)
	q.Config = &f.before
	f.rev = q.BaseRevision
	r, e := f.s.Do(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	f.id = r.ExperimentID
	// More than 64 targets enroll over bounded reconciliation batches.
	for i := 0; i < n/64+1; i++ {
		if e = f.s.Tick(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	f.verify(f.panels...)
	return f
}
func (f *tuningFixture) verify(panels ...string) {
	f.t.Helper()
	for _, p := range panels {
		a, e := f.s.Load(context.Background(), p)
		if e != nil {
			f.t.Fatal(e)
		}
		if e = f.s.Applied(context.Background(), p, a.Generation); e != nil {
			f.t.Fatal(e)
		}
		sqlMust(f.t, f.db, "UPDATE panel_routing_state SET state='APPLIED',revision=$3,performance_generation=$2,verified_at=now() WHERE panel_id=$1", p, a.Generation, f.rev)
	}
}
func (f *tuningFixture) read() (int64, *residentialperf.Tuning) {
	f.t.Helper()
	var version int64
	var b []byte
	if e := f.db.QueryRow("SELECT version,tuning FROM residential_performance_experiments WHERE id=$1", f.id).Scan(&version, &b); e != nil {
		f.t.Fatal(e)
	}
	var x *residentialperf.Tuning
	if len(b) > 0 {
		if e := json.Unmarshal(b, &x); e != nil {
			f.t.Fatal(e)
		}
	}
	return version, x
}
func (f *tuningFixture) q(action string) residentialperf.Request {
	v, x := f.read()
	q := residentialperf.Request{RequestID: perfUUID(), ExperimentID: f.id, ExpectedVersion: v, Action: action}
	if x != nil {
		q.TuningID = x.ID
	}
	return q
}
func (f *tuningFixture) startq() residentialperf.Request {
	q := f.q("tune_start")
	q.TuningID = ""
	q.PanelIDs = f.panels[:1]
	q.Config = &f.candidate
	q.Minutes = 15
	q.BaseRevision = f.rev
	q.BasePlan = "before"
	return q
}
func (f *tuningFixture) start() residentialperf.Request {
	f.t.Helper()
	q := f.startq()
	if _, e := f.s.Do(context.Background(), q); e != nil {
		f.t.Fatal(e)
	}
	return q
}
func (f *tuningFixture) do(action string) {
	f.t.Helper()
	if _, e := f.s.Do(context.Background(), f.q(action)); e != nil {
		f.t.Fatal(action, e)
	}
}
func (f *tuningFixture) tick() {
	f.t.Helper()
	if e := f.s.Tick(context.Background()); e != nil {
		f.t.Fatal(e)
	}
}
func (f *tuningFixture) phase(want string) {
	f.t.Helper()
	_, x := f.read()
	if x == nil || x.Phase != want {
		f.t.Fatalf("phase want %s got %+v", want, x)
	}
}
func (f *tuningFixture) config(panel string, want residentialperf.Config, gen int64) {
	f.t.Helper()
	a, e := f.s.Load(context.Background(), panel)
	actual, _ := json.Marshal(a.Config)
	expected, _ := json.Marshal(want)
	if e != nil || string(actual) != string(expected) || a.Generation != gen {
		f.t.Fatalf("assignment mismatch gen=%d want=%d config=%s error=%v", a.Generation, gen, actual, e)
	}
}

func TestTuningSelectedIsolationIdempotencyAndExactCancellation(t *testing.T) {
	f := newTuningFixture(t, 2)
	ctx := context.Background()
	q := f.start()
	first, e := f.s.Do(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := f.s.Do(ctx, q)
	if e != nil || replay != first {
		t.Fatal("lost response", e)
	}
	changed := q
	changed.Minutes = 20
	if _, e = f.s.Do(ctx, changed); e == nil {
		t.Fatal("changed payload reused receipt")
	}
	f.config(f.panels[0], f.candidate, 2)
	f.config(f.panels[1], f.before, 1)
	future := fleetSeed(t, f.db, "future")
	f.tick()
	f.config(future, f.before, 1)
	for _, action := range []string{"publish", "keep", "promote", "tune_publish"} {
		q := f.q(action)
		q.Scope = "fleet"
		if _, e = f.s.Do(ctx, q); e == nil {
			t.Fatal("unverified/incompatible action accepted", action)
		}
	}
	qnew := fleetRequest(t, f.db, "fleet", nil)
	if _, e = f.s.Do(ctx, qnew); e == nil {
		t.Fatal("single-open-profile bypass")
	}
	f.do("tune_cancel")
	f.phase("RESTORING")
	f.config(f.panels[0], f.before, 3)
	if e = f.s.Applied(ctx, f.panels[0], 2); e == nil {
		t.Fatal("stale acknowledgement accepted")
	}
	f.tick()
	f.phase("RESTORING")
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
	f.config(f.panels[1], f.before, 1)
	f.config(future, f.before, 1)
	var before []byte
	if e = f.db.QueryRow("SELECT before_config FROM residential_performance_targets WHERE experiment_id=$1 AND panel_id=$2", f.id, f.panels[0]).Scan(&before); e != nil || len(before) > 0 {
		t.Fatal("original full-rollback snapshot changed", e)
	}
	// Historical operation replay remains historical; callers must read current state.
	replay, e = f.s.Do(ctx, q)
	if e != nil || replay != first {
		t.Fatal("historical receipt changed", e)
	}
}

func TestTuningRejectsUnrelatedConfigAndConcurrentOperators(t *testing.T) {
	f := newTuningFixture(t, 2)
	ctx := context.Background()
	q := f.startq()
	c := f.candidate
	c.FastOpen = false
	q.Config = &c
	if _, e := f.s.Do(ctx, q); e == nil {
		t.Fatal("TFO silently replaced")
	}
	a, b := f.startq(), f.startq()
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	for _, q := range []residentialperf.Request{a, b} {
		go func(q residentialperf.Request) { defer wg.Done(); _, e := f.s.Do(ctx, q); errs <- e }(q)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("concurrent version/CAS", success)
	}
	f.do("tune_cancel")
	f.verify(f.panels[0])
	f.tick()
	old := f.q("tune_restore")
	old.TuningID = perfUUID()
	if _, e := f.s.Do(ctx, old); e == nil {
		t.Fatal("wrong trial identity")
	}
}

func TestTuningExpiryRecoverySurvivesEnrollmentFailureAndRestart(t *testing.T) {
	f := newTuningFixture(t, 2)
	f.start()
	future := fleetSeed(t, f.db, "broken-future")
	sqlMust(t, f.db, "CREATE FUNCTION deny_enrollment() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected enrollment failure'; END $$")
	sqlMust(t, f.db, "CREATE TRIGGER deny_enrollment BEFORE INSERT ON residential_performance_targets FOR EACH ROW EXECUTE FUNCTION deny_enrollment()")
	sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(clock_timestamp()-interval '1 second')) WHERE id=$1", f.id)
	f.s = residentialperf.Store{DB: f.db}
	if e := f.s.Tick(context.Background()); e == nil {
		t.Fatal("fault injection missing")
	}
	f.phase("RESTORING")
	f.config(f.panels[0], f.before, 3)
	f.verify(f.panels[0])
	if e := f.s.Tick(context.Background()); e == nil {
		t.Fatal("enrollment should still fail")
	}
	f.phase("RESTORED")
	sqlMust(t, f.db, "DROP TRIGGER deny_enrollment ON residential_performance_targets")
	f.tick()
	f.config(future, f.before, 1)
}

func TestTuningConflictRetainsRecoveryWithoutPartialOverwrite(t *testing.T) {
	f := newTuningFixture(t, 2)
	q := f.startq()
	q.PanelIDs = f.panels
	q.BasePlan = ""
	if _, e := f.s.Do(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, f.db, "UPDATE residential_performance_panels SET generation=generation+1 WHERE panel_id=$1", f.panels[1])
	sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(clock_timestamp()-interval '1 second')) WHERE id=$1", f.id)
	f.tick()
	_, x := f.read()
	if x.Phase != "RESTORING" || x.Assigned || x.Conflict == "" {
		t.Fatal("lost conflict obligation", x)
	}
	f.config(f.panels[0], f.candidate, 2)
	f.config(f.panels[1], f.candidate, 3)
	// Test-only repair of injected drift, followed by ordinary durable recovery.
	sqlMust(t, f.db, "UPDATE residential_performance_panels SET generation=generation-1 WHERE panel_id=$1", f.panels[1])
	f.tick()
	f.config(f.panels[0], f.before, 3)
	f.config(f.panels[1], f.before, 3)
	f.verify(f.panels...)
	f.tick()
	f.phase("RESTORED")
}

func TestTuningFailuresRemainRejectedAfterSuccessfulAcknowledgement(t *testing.T) {
	f := newTuningFixture(t, 1)
	f.start()
	f.s.Failed(context.Background(), f.panels[0], 2, errors.New("injected native timeout"))
	f.verify(f.panels[0])
	_, x := f.read()
	if !x.Rejected || x.Phase != "RESTORING" {
		t.Fatal("successful ack erased rejection", x)
	}
	if _, e := f.s.Do(context.Background(), f.q("tune_publish")); e == nil {
		t.Fatal("rejected trial published")
	}
	f.tick()
	f.config(f.panels[0], f.before, 3)
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
	_, x = f.read()
	if !x.Rejected {
		t.Fatal("lost rejection history")
	}
}

func TestTuningFleetPublishRestoreMoreThan64AndFutureEnrollment(t *testing.T) {
	f := newTuningFixture(t, 66)
	f.start()
	f.verify(f.panels[0])
	f.do("tune_publish")
	f.phase("PUBLISHING")
	if _, e := f.s.Do(context.Background(), f.startq()); e == nil {
		t.Fatal("new trial hid pending fleet application")
	}
	for _, p := range f.panels {
		f.config(p, f.candidate, 2)
	}
	future := fleetSeed(t, f.db, "after-publish")
	f.tick()
	f.config(future, f.candidate, 1)
	f.verify(f.panels...)
	f.tick()
	f.phase("PUBLISHING")
	f.verify(future)
	f.tick()
	f.phase("PUBLISHED")
	f.do("tune_restore")
	f.phase("RESTORING")
	for _, p := range f.panels {
		f.config(p, f.before, 3)
	}
	f.config(future, f.before, 2)
	later := fleetSeed(t, f.db, "during-restore")
	f.tick()
	f.config(later, f.before, 1)
	f.verify(f.panels...)
	f.verify(future)
	f.tick()
	f.phase("RESTORING")
	f.verify(later)
	f.tick()
	f.phase("RESTORED")
	// Beginning a new trial supersedes only a completed latest recovery point.
	f.verify(f.panels...)
	f.verify(future, later)
	f.start()
	f.phase("TESTING")
}

func TestTuningPublicationRechecksWallClockAfterLockWait(t *testing.T) {
	f := newTuningFixture(t, 1)
	f.start()
	f.verify(f.panels[0])
	sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(clock_timestamp()+interval '60.6 seconds')) WHERE id=$1", f.id)
	q := f.q("tune_publish")
	tx, e := f.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = residentialperf.Lock(context.Background(), tx); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := f.s.Do(context.Background(), q); done <- e }()
	time.Sleep(800 * time.Millisecond)
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil || !strings.Contains(e.Error(), "cutoff") {
		t.Fatal("lock-wait wall clock authorization", e)
	}
	f.phase("TESTING")
}

func TestTuningParentRollbackSupersedesEveryPhase(t *testing.T) {
	for _, phase := range []string{"TESTING", "PUBLISHING", "PUBLISHED", "RESTORING"} {
		t.Run(phase, func(t *testing.T) {
			f := newTuningFixture(t, 2)
			f.start()
			if phase == "PUBLISHING" || phase == "PUBLISHED" {
				f.verify(f.panels[0])
				f.do("tune_publish")
			}
			if phase == "PUBLISHED" {
				f.verify(f.panels...)
				f.tick()
			}
			if phase == "RESTORING" {
				f.do("tune_cancel")
			}
			f.phase(phase)
			f.do("rollback")
			f.phase("SUPERSEDED")
			f.tick()
			for _, p := range f.panels {
				a, e := f.s.Load(context.Background(), p)
				if e != nil || a.Config != nil {
					t.Fatal("full rollback resurrected tuning", e)
				}
			}
			f.verify(f.panels...)
			f.tick()
			var state string
			f.db.QueryRow("SELECT state FROM residential_performance_experiments WHERE id=$1", f.id).Scan(&state)
			if state != "ROLLED_BACK" {
				t.Fatal("parent rollback incomplete", state)
			}
		})
	}
}
func TestTuningUnavailableSurvivorAndDeletedTarget(t *testing.T) {
	f := newTuningFixture(t, 2)
	q := f.startq()
	q.PanelIDs = f.panels
	q.BasePlan = ""
	if _, e := f.s.Do(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	f.do("tune_cancel")
	f.verify(f.panels[0])
	sqlMust(t, f.db, "UPDATE panel_instances SET enabled=false WHERE id=$1", f.panels[1])
	f.tick()
	f.phase("RESTORING")
	sqlMust(t, f.db, "UPDATE droplets SET state='DELETED' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)", f.panels[1])
	f.tick()
	f.phase("RESTORED")
}
func TestTuningMigrationRefusesActiveRecoveryDowngrade(t *testing.T) {
	f := newTuningFixture(t, 1)
	f.start()
	down, e := os.ReadFile("../../migrations/000165_residential_tuning.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(string(down)); e == nil {
		t.Fatal("active tuning state removed")
	}
	f.do("tune_cancel")
	f.verify(f.panels[0])
	f.tick()
	if _, e = f.db.Exec(string(down)); e != nil {
		t.Fatal("settled additive migration", e)
	}
}

func TestTuningExplicitFleetRestoreConflictPersistsIntent(t *testing.T) {
	f := newTuningFixture(t, 2)
	f.start()
	f.verify(f.panels[0])
	f.do("tune_publish")
	f.verify(f.panels...)
	f.tick()
	f.phase("PUBLISHED")
	sqlMust(t, f.db, "UPDATE residential_performance_panels SET generation=generation+1 WHERE panel_id=$1", f.panels[1])
	f.do("tune_restore")
	_, x := f.read()
	if x.Phase != "RESTORING" || x.Assigned || x.Conflict == "" {
		t.Fatal("explicit restore intent lost", x)
	}
	f.config(f.panels[0], f.candidate, 2)
	future := fleetSeed(t, f.db, "paused-enrollment")
	f.s = residentialperf.Store{DB: f.db}
	f.tick()
	a, e := f.s.Load(context.Background(), future)
	if e != nil || a.Generation != 0 {
		t.Fatal("conflicted candidate enrollment continued", e)
	}
	sqlMust(t, f.db, "UPDATE residential_performance_panels SET generation=generation-1 WHERE panel_id=$1", f.panels[1])
	f.tick()
	f.config(f.panels[0], f.before, 3)
	f.config(future, f.before, 1)
	f.verify(f.panels...)
	f.verify(future)
	f.tick()
	f.phase("RESTORED")
}
func TestTuningIdentityDriftCannotShrinkProofSet(t *testing.T) {
	for _, scope := range []string{"selected", "fleet"} {
		for _, marker := range []string{"NULL", "'11111111-1111-4111-8111-111111111111'::uuid"} {
			t.Run(scope+marker, func(t *testing.T) {
				f := newTuningFixture(t, 2)
				f.start()
				if scope == "fleet" {
					f.verify(f.panels[0])
					f.do("tune_publish")
					f.verify(f.panels...)
				} else {
					f.do("tune_cancel")
					f.verify(f.panels[0])
				}
				_, x := f.read()
				panel := f.panels[0]
				if scope == "fleet" {
					panel = f.panels[1]
				}
				sqlMust(t, f.db, "UPDATE residential_performance_targets SET tuning_id="+marker+" WHERE experiment_id=$1 AND panel_id=$2", f.id, panel)
				f.tick()
				_, state := f.read()
				want := "RESTORING"
				if scope == "fleet" {
					want = "PUBLISHING"
				}
				if state.Phase != want || state.Conflict == "" {
					t.Fatal("missing identity silently omitted", state)
				}
				sqlMust(t, f.db, "UPDATE residential_performance_targets SET tuning_id=$3 WHERE experiment_id=$1 AND panel_id=$2", f.id, panel, x.ID)
				f.tick()
				want = "RESTORED"
				if scope == "fleet" {
					want = "PUBLISHED"
				}
				f.phase(want)
			})
		}
	}
}

func TestTuningLostFleetTargetRecordBlocksCompletionAndOverwrite(t *testing.T) {
	f := newTuningFixture(t, 2)
	f.start()
	f.verify(f.panels[0])
	f.do("tune_publish")
	f.verify(f.panels...)
	sqlMust(t, f.db, "DELETE FROM residential_performance_targets WHERE experiment_id=$1 AND panel_id=$2", f.id, f.panels[1])
	// Tick must not complete publication. Ordinary enrollment must not be allowed
	// to replace a lost rollback record with a new snapshot.
	if e := f.s.Tick(context.Background()); e == nil {
		t.Fatal("orphan enrollment was not blocked")
	}
	_, x := f.read()
	if x.Phase != "PUBLISHING" || x.Conflict == "" {
		t.Fatal("orphaned assignment omitted from proof", x)
	}
}

func TestTuningDeploymentGuardRejectsOverlayUnawareRollback(t *testing.T) {
	f := newTuningFixture(t, 1)
	f.start()
	script, e := os.ReadFile("../../deploy/service-topology.sh")
	if e != nil {
		t.Fatal(e)
	}
	start := strings.Index(string(script), "import subprocess,runpy,sys,pathlib")
	end := strings.Index(string(script)[start:], "\nGUARD")
	if start < 0 || end < 0 {
		t.Fatal("deployment guard missing")
	}
	guard := string(script)[start : start+end]
	var schema string
	if e = f.db.QueryRow("SELECT current_schema()").Scan(&schema); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(os.Getenv("BULK_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	params := u.Query()
	params.Del("search_path")
	params.Del("options")
	u.RawQuery = params.Encode()
	dir := t.TempDir()
	if e = os.WriteFile(filepath.Join(dir, "env"), []byte("DATABASE_URL=\""+u.String()+"\"\nMASTER_KEY_FILE=/tmp/unused\nMASTER_KEY_VERSION=1\nHTTP_ADDR=127.0.0.1:0\n"), 0600); e != nil {
		t.Fatal(e)
	}
	bootstrap, e := filepath.Abs("../../deploy/bootstrap.py")
	if e != nil {
		t.Fatal(e)
	}
	run := func() error {
		cmd := exec.Command("python3", "-c", guard, bootstrap, dir)
		cmd.Env = append(os.Environ(), "PGOPTIONS=-c search_path="+schema)
		b, e := cmd.CombinedOutput()
		if e != nil {
			return fmt.Errorf("guard: %w: %s", e, b)
		}
		return nil
	}
	if e = run(); e == nil || !strings.Contains(e.Error(), "Unresolved residential tuning") {
		t.Fatal("active trial guard did not reject for tuning", e)
	}
	f.verify(f.panels[0])
	f.do("tune_publish")
	if e = run(); e == nil || !strings.Contains(e.Error(), "Unresolved residential tuning") {
		t.Fatal("publishing guard", e)
	}
	f.do("tune_restore")
	if e = run(); e == nil || !strings.Contains(e.Error(), "Unresolved residential tuning") {
		t.Fatal("active restoration guard did not reject for tuning", e)
	}
	f.verify(f.panels[0])
	f.tick()
	if e = run(); e != nil {
		t.Fatal("settled recovery blocked rollback", e)
	}
}

func TestTuningParentRollbackCannotOmitLostRecoveryTarget(t *testing.T) {
	for _, when := range []string{"before", "during"} {
		t.Run(when, func(t *testing.T) {
			f := newTuningFixture(t, 2)
			f.start()
			if when == "during" {
				f.do("rollback")
			}
			sqlMust(t, f.db, "DELETE FROM residential_performance_targets WHERE experiment_id=$1 AND panel_id=$2", f.id, f.panels[0])
			if when == "before" {
				if _, e := f.s.Do(context.Background(), f.q("rollback")); e == nil {
					t.Fatal("rollback ignored orphan")
				}
				f.phase("TESTING")
				f.config(f.panels[0], f.candidate, 2)
				f.config(f.panels[1], f.before, 1)
			} else {
				f.verify(f.panels...)
				f.tick()
				var state, reason string
				if e := f.db.QueryRow("SELECT state,reason FROM residential_performance_experiments WHERE id=$1", f.id).Scan(&state, &reason); e != nil {
					t.Fatal(e)
				}
				if state != "ROLLING_BACK" || !strings.Contains(reason, "missing target") {
					t.Fatal("false full rollback completion", state, reason)
				}
			}
		})
	}
}
