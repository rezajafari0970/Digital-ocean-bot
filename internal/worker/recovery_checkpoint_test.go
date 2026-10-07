package worker

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

const recoveryAccount = "00000000-0000-0000-0000-000000000001"
const recoveryID = "00000000-0000-0000-0000-000000000002"

type recoveryProbe struct {
	operations, deployments, bypasses atomic.Int32
	outcome                           error
	run                               func(context.Context) error
}

func (p *recoveryProbe) RecoverOperation(ctx context.Context, _ RecoveryItem) error {
	p.operations.Add(1)
	if p.run != nil {
		return p.run(ctx)
	}
	return p.outcome
}
func (p *recoveryProbe) RecoverDeployment(ctx context.Context, _ RecoveryItem) error {
	p.deployments.Add(1)
	if p.run != nil {
		return p.run(ctx)
	}
	return p.outcome
}
func (p *recoveryProbe) BypassDeploymentBackoff(context.Context, RecoveryItem) bool {
	p.bypasses.Add(1)
	return true
}
func recoveryFixture(t *testing.T, kind string) (*sql.DB, *recoveryProbe, Worker) {
	t.Helper()
	db := failureLedgerFixture(t)
	up, e := os.ReadFile("../../migrations/000162_worker_recovery_checkpoints.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(up)); e != nil {
		t.Fatal(e)
	}
	fixture := `CREATE TABLE accounts(id uuid,provider_state text);
 CREATE TABLE operations(id uuid,account_id uuid,kind text,state text,updated_at timestamptz);
 CREATE TABLE deployments(id uuid,account_id uuid,state text,current_step text,profile_snapshot jsonb,installer_generation int,updated_at timestamptz);
 CREATE TABLE deployment_installer_selections(deployment_id uuid,generation int);
 CREATE TABLE installer_runs(deployment_id uuid,generation int,state text,manifest_snapshot jsonb);
 CREATE TABLE droplets(id uuid,state text);
 INSERT INTO accounts VALUES('00000000-0000-0000-0000-000000000001','ACTIVE');`
	if _, e = db.Exec(fixture); e != nil {
		t.Fatal(e)
	}
	if kind == "operation" {
		_, e = db.Exec("INSERT INTO operations VALUES($1,$2,'CREATE_DROPLET','running',now()-interval '1 minute')", recoveryID, recoveryAccount)
	} else {
		_, e = db.Exec(`INSERT INTO deployments VALUES($1,$2,'WAITING_INSTALLER','provision','{"installer_ref":{}}',1,now()-interval '1 minute')`, recoveryID, recoveryAccount)
	}
	if e != nil {
		t.Fatal(e)
	}
	p := &recoveryProbe{}
	return db, p, Worker{Store: RecoveryStore{DB: db}, Failures: FailureStore{DB: db}, Handler: p, Interval: 5 * time.Millisecond}
}
func sqlMust(t *testing.T, db *sql.DB, s string) {
	t.Helper()
	if _, e := db.Exec(s); e != nil {
		t.Fatal(e)
	}
}
func checkpointCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if e := db.QueryRow("SELECT count(*) FROM worker_recovery_checkpoints").Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func calls(p *recoveryProbe) int32 { return p.operations.Load() + p.deployments.Load() }
func waitRecovery(t *testing.T, fn func() bool) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("recovery condition timed out")
}
func TestRecoveryLedgerReadFaultNeverCallsBypassOrHandler(t *testing.T) {
	for _, kind := range []string{"operation", "deployment"} {
		t.Run(kind, func(t *testing.T) {
			db, p, w := recoveryFixture(t, kind)
			sqlMust(t, db, "ALTER TABLE worker_item_failures RENAME COLUMN next_retry_at TO unavailable_retry")
			if e := w.Once(context.Background()); e == nil {
				t.Fatal("read fault hidden")
			}
			if calls(p) != 0 || p.bypasses.Load() != 0 {
				t.Fatal("read fault admitted work", calls(p), p.bypasses.Load())
			}
		})
	}
}
func TestRecoveryMissingCheckpointOrFailedPruningRefusesWork(t *testing.T) {
	t.Run("checkpoint", func(t *testing.T) {
		db, p, w := recoveryFixture(t, "deployment")
		sqlMust(t, db, "DROP TABLE worker_recovery_checkpoints")
		if w.Once(context.Background()) == nil || calls(p) != 0 {
			t.Fatal("missing checkpoint table admitted work")
		}
	})
	t.Run("pruning", func(t *testing.T) {
		db, p, w := recoveryFixture(t, "deployment")
		sqlMust(t, db, "ALTER TABLE worker_item_failures RENAME COLUMN kind TO unavailable_kind")
		if w.Once(context.Background()) == nil || calls(p) != 0 || p.bypasses.Load() != 0 {
			t.Fatal("failed prune/read hidden")
		}
	})
}
func TestRecoveryCompletionFaultPreservesDurableFenceAndAtomicBackoff(t *testing.T) {
	for _, fault := range []string{"failure_insert", "failure_update", "failure_clear", "checkpoint_delete", "checkpoint_insert"} {
		t.Run(fault, func(t *testing.T) {
			db, p, w := recoveryFixture(t, "deployment")
			p.outcome = errors.New("native failure")
			if fault == "failure_update" || fault == "failure_clear" {
				sqlMust(t, db, "INSERT INTO worker_item_failures(kind,item_id,failures,next_retry_at,last_error) VALUES('deployment','"+recoveryID+"',2,now()-interval '1 minute','previous')")
			}
			if fault == "failure_clear" {
				p.outcome = nil
			}
			table, event := "worker_item_failures", "INSERT"
			switch fault {
			case "failure_update":
				event = "UPDATE"
			case "failure_clear":
				event = "DELETE"
			case "checkpoint_delete":
				table, event = "worker_recovery_checkpoints", "DELETE"
			case "checkpoint_insert":
				table, event = "worker_recovery_checkpoints", "INSERT"
			}
			sqlMust(t, db, "CREATE FUNCTION reject_recovery_write() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'recovery test fault'; END$$; CREATE TRIGGER reject_recovery_write BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION reject_recovery_write()")
			if e := w.Once(context.Background()); e == nil {
				t.Fatal("persistence failure hidden")
			}
			expected := int32(1)
			if fault == "checkpoint_insert" {
				expected = 0
			}
			if calls(p) != expected || checkpointCount(t, db) != int(expected) {
				t.Fatal("handler/checkpoint mismatch", calls(p), checkpointCount(t, db))
			}
			if fault == "checkpoint_delete" {
				var n int
				db.QueryRow("SELECT count(*) FROM worker_item_failures").Scan(&n)
				if n != 0 {
					t.Fatal("partial completion committed")
				}
			}
			if fault != "checkpoint_insert" {
				// A new Worker value simulates loss of local memory. Persisted checkpoint
				// must prevent replay, even with a permissive installer bypass.
				p2 := &recoveryProbe{outcome: p.outcome}
				w2 := w
				w2.Handler = p2
				_ = w2.Once(context.Background())
				if calls(p2) != 0 {
					t.Fatal("remote handler replayed across worker reconstruction")
				}
			}
			sqlMust(t, db, "DROP TRIGGER reject_recovery_write ON "+table)
			if fault == "checkpoint_insert" {
				if e := w.Once(context.Background()); e != nil {
					t.Fatal(e)
				}
				if calls(p) != 1 {
					t.Fatal("did not recover admission")
				}
				return
			}
			if e := w.Once(context.Background()); e != nil {
				t.Fatal(e)
			}
			if calls(p) != 1 || checkpointCount(t, db) != 0 {
				t.Fatal("reconciliation replayed handler")
			}
			var reason string
			var next time.Time
			if e := db.QueryRow("SELECT last_error,next_retry_at FROM worker_item_failures WHERE kind='deployment' AND item_id=$1", recoveryID).Scan(&reason, &next); e != nil {
				t.Fatal(e)
			}
			if reason != (recoveryInterrupted{}).Error() || time.Until(next) < 20*time.Second {
				t.Fatal("interrupted outcome did not persist conservative backoff")
			}
		})
	}
}
func TestRecoveryLiveOwnerCrashAndSessionLoss(t *testing.T) {
	for _, mode := range []string{"close", "backend_loss", "unlock"} {
		t.Run(mode, func(t *testing.T) {
			db, p, w := recoveryFixture(t, "deployment")
			ctx := context.Background()
			l, ok, e := acquireRecoveryLease(ctx, db, "deployment", recoveryID)
			if e != nil || !ok {
				t.Fatal(e)
			}
			defer l.close()
			if e = l.begin(ctx, recoveryAccount); e != nil {
				t.Fatal(e)
			}
			if e = w.Once(ctx); e != nil || calls(p) != 0 || checkpointCount(t, db) != 1 {
				t.Fatal("live owner reaped", e)
			}
			switch mode {
			case "backend_loss":
				var ended bool
				if e = db.QueryRow("SELECT pg_terminate_backend($1)", l.backend).Scan(&ended); e != nil || !ended {
					t.Fatal(e)
				}
			case "unlock":
				if _, e = l.conn.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); e != nil {
					t.Fatal(e)
				}
			case "close":
				l.close()
			}
			if e = l.complete(ctx, recoveryAccount, nil); e == nil {
				t.Fatal("lost owner completed checkpoint")
			}
			// In-process active handlers remain fenced even after their SQL session dies.
			if e = reconcileRecoveryCheckpoints(ctx, db, func(string) bool { return true }, "recovery"); e != nil || checkpointCount(t, db) != 1 {
				t.Fatal("active lane reaped", e)
			}
			if e = w.Once(ctx); e != nil {
				t.Fatal(e)
			}
			if calls(p) != 0 || p.bypasses.Load() != 0 || checkpointCount(t, db) != 0 {
				t.Fatal("orphan reconciliation replayed native work")
			}
		})
	}
}
func TestRecoveryConcurrentWorkersDoNotOverlapLiveHandler(t *testing.T) {
	db, p, w := recoveryFixture(t, "deployment")
	entered, release := make(chan struct{}), make(chan struct{})
	p.run = func(context.Context) error { close(entered); <-release; return nil }
	done := make(chan error, 1)
	go func() { done <- w.Once(context.Background()) }()
	<-entered
	if e := w.Once(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls(p) != 1 || checkpointCount(t, db) != 1 {
		t.Fatal("concurrent duplicate")
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if checkpointCount(t, db) != 0 {
		t.Fatal("checkpoint leaked")
	}
}
func TestRecoveryAsyncPersistenceFailureInvalidatesProgressUntilReconciled(t *testing.T) {
	db, p, w := recoveryFixture(t, "deployment")
	p.outcome = errors.New("native failure")
	sqlMust(t, db, "CREATE FUNCTION reject_async() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'async fixture fault'; END$$; CREATE TRIGGER reject_async BEFORE INSERT ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION reject_async()")
	var stamp atomic.Int64
	stamp.Store(-1)
	w.Progress = func(v time.Time) { stamp.Store(v.Unix()) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	waitRecovery(t, func() bool { return calls(p) == 1 && stamp.Load() == 0 })
	time.Sleep(30 * time.Millisecond)
	if calls(p) != 1 || stamp.Load() != 0 {
		t.Fatal("async fault hidden or replayed")
	}
	sqlMust(t, db, "DROP TRIGGER reject_async ON worker_item_failures")
	waitRecovery(t, func() bool { return stamp.Load() > 0 && checkpointCount(t, db) == 0 })
	if calls(p) != 1 {
		t.Fatal("recovery replayed before persisted backoff")
	}
	cancel()
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestRecoveryProgressGenerationRejectsOlderSuccess(t *testing.T) {
	var stamp int64
	p := recoveryProgress{publish: func(v time.Time) { stamp = v.Unix() }}
	old := p.start(nil)
	p.fail("")
	p.success(old)
	if stamp != 0 {
		t.Fatal("stale success hid fault")
	}
	p.success(p.start(nil))
	if stamp <= 0 {
		t.Fatal("successful later scan did not recover")
	}
}
func TestRecoveryCancellationPersistsBeforeReleasingLane(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	p.run = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	done := make(chan error, 1)
	go func() { done <- w.Once(ctx) }()
	<-entered
	cancel()
	if e := <-done; e != nil && !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if checkpointCount(t, db) != 0 {
		t.Fatal("canceled completion not persisted")
	}
	var n int
	if e := db.QueryRow("SELECT failures FROM worker_item_failures WHERE item_id=$1", recoveryID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
func TestRecoveryCheckpointMigrationRefusesUnsafeDowngrade(t *testing.T) {
	db, _, _ := recoveryFixture(t, "operation")
	down, e := os.ReadFile("../../migrations/000162_worker_recovery_checkpoints.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) VALUES('operation','"+recoveryID+"','"+recoveryAccount+"')")
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("dropped unresolved checkpoint")
	}
	sqlMust(t, db, "DELETE FROM worker_recovery_checkpoints")
	if _, e = db.Exec(string(down)); e != nil {
		t.Fatal(e)
	}
}

func TestRecoveryFailedActiveLaneCannotRestoreReadiness(t *testing.T) {
	var stamp int64
	p := recoveryProgress{publish: func(v time.Time) { stamp = v.Unix() }}
	p.fail("deployment:pending")
	p.success(p.start(func(string) bool { return true }))
	if stamp != 0 {
		t.Fatal("active failed completion hidden by newer scan")
	}
	// Only a scan after the failed lane exits can reconcile its durable outcome.
	p.success(p.start(func(string) bool { return false }))
	if stamp <= 0 {
		t.Fatal("later reconciled scan did not restore progress")
	}
}
