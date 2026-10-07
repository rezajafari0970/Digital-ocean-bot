package worker

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"sync/atomic"
	"testing"
	"time"
)

func TestLifecycleCompletionFailureSurvivesRestartAndInvalidatesProgress(t *testing.T) {
	for _, fault := range []string{"failure_insert", "failure_clear", "checkpoint_delete"} {
		t.Run(fault, func(t *testing.T) {
			db, _, _ := recoveryFixture(t, "operation")
			table, event := "worker_item_failures", "INSERT"
			var outcome error = errors.New("native lifecycle failure")
			if fault == "failure_clear" {
				event = "DELETE"
				outcome = nil
				sqlMust(t, db, "INSERT INTO worker_item_failures(kind,item_id,failures,next_retry_at,last_error) VALUES('lifecycle','"+recoveryID+"',1,now()-interval '1 minute','old')")
			}
			if fault == "checkpoint_delete" {
				table, event = "worker_recovery_checkpoints", "DELETE"
			}
			sqlMust(t, db, "CREATE FUNCTION reject_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'lifecycle fixture fault'; END$$; CREATE TRIGGER reject_lifecycle BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION reject_lifecycle()")
			var n, failed atomic.Int32
			var stamp atomic.Int64
			stamp.Store(-1)
			w := LifecycleWorker{Failures: FailureStore{DB: db}, Store: lifecycleSourceStub{[]droplets.LifecycleItem{{ID: recoveryID, AccountID: recoveryAccount}}}, Interval: 5 * time.Millisecond,
				Handler: lifecycleHandlerFunc(func(context.Context, droplets.LifecycleItem) error { n.Add(1); return outcome }),
				Progress: func(v time.Time) {
					stamp.Store(v.Unix())
					if v.Unix() == 0 {
						failed.Add(1)
					}
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- w.Run(ctx) }()
			waitRecovery(t, func() bool { return failed.Load() > 0 })
			// A DELETE fault can be repaired through an interrupted INSERT/UPDATE;
			// either remaining unhealthy or recovered with durable delay is correct.
			time.Sleep(25 * time.Millisecond)
			cancel()
			<-done
			if n.Load() != 1 {
				t.Fatal("failed lifecycle completion replayed", n.Load())
			}
			restarted := w
			restarted.Progress = nil
			_ = restarted.Once(context.Background())
			if n.Load() != 1 {
				t.Fatal("reconstructed lifecycle replayed pending completion")
			}
			sqlMust(t, db, "DROP TRIGGER reject_lifecycle ON "+table)
			// Advance the durable reaper retry deadline after repairing the injected fault.
			sqlMust(t, db, "UPDATE worker_recovery_checkpoints SET reconcile_after=now()")
			if err := restarted.Once(context.Background()); err != nil {
				t.Fatal(err)
			}
			if n.Load() != 1 || checkpointCount(t, db) != 0 {
				t.Fatal("reconciliation replayed lifecycle")
			}
			var reason string
			if err := db.QueryRow("SELECT last_error FROM worker_item_failures WHERE kind='lifecycle' AND item_id=$1", recoveryID).Scan(&reason); err != nil || reason != (recoveryInterrupted{}).Error() {
				t.Fatal(reason, err)
			}
		})
	}
}
func TestRecoveryReaperDoesNotTouchLifecycleLane(t *testing.T) {
	db, _, w := recoveryFixture(t, "operation")
	ctx := context.Background()
	l, ok, err := acquireRecoveryLease(ctx, db, "lifecycle", recoveryID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = l.begin(ctx, recoveryAccount); err != nil {
		t.Fatal(err)
	}
	l.close()
	if err = w.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if checkpointCount(t, db) != 1 {
		t.Fatal("recovery module reaped lifecycle")
	}
	if err = reconcileRecoveryCheckpoints(ctx, db, func(string) bool { return true }, "lifecycle"); err != nil {
		t.Fatal(err)
	}
	if checkpointCount(t, db) != 1 {
		t.Fatal("active lifecycle reaped")
	}
	if err = reconcileRecoveryCheckpoints(ctx, db, nil, "lifecycle"); err != nil {
		t.Fatal(err)
	}
	if checkpointCount(t, db) != 0 {
		t.Fatal("orphan lifecycle not reconciled")
	}
}
