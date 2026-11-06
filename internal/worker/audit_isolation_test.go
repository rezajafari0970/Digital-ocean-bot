package worker

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuditBatchPanicDoesNotStopOtherItems(t *testing.T) {
	for _, custom := range []bool{false, true} {
		var calls [8]atomic.Int32
		var admit func(context.Context, func(context.Context) error) error
		if custom {
			admit = func(c context.Context, f func(context.Context) error) error { return f(c) }
		}
		err := RunBoundedBatch(context.Background(), len(calls), 3, admit, func(_ context.Context, i int) error {
			calls[i].Add(1)
			if i == 2 {
				panic("private account credential")
			}
			return nil
		})
		if !errors.Is(err, resilience.ErrTaskPanic) || strings.Contains(err.Error(), "private account credential") {
			t.Fatal("missing or unsafe task failure")
		}
		for i := range calls {
			if calls[i].Load() != 1 {
				t.Fatalf("item %d ran %d times", i, calls[i].Load())
			}
		}
	}
}

func TestAuditModulePanicCancelsAndJoinsBeforeReturn(t *testing.T) {
	var modules Modules
	started, joined := make(chan struct{}), make(chan struct{})
	modules.Add(RoleControl, "sibling", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		close(joined)
	})
	modules.Add(RoleControl, "broken", func(context.Context) { <-started; panic("private module credential") })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := modules.Run(ctx, RoleControl)
	if !errors.Is(err, resilience.ErrTaskPanic) || strings.Contains(err.Error(), "private module credential") {
		t.Fatal("missing or unsafe module failure")
	}
	select {
	case <-joined:
	default:
		t.Fatal("module owner returned before sibling cleanup")
	}
}

func TestAuditRecoveryPanicPersistsBackoffWithoutReplayPostgres(t *testing.T) {
	for _, kind := range []string{"operation", "deployment"} {
		t.Run(kind, func(t *testing.T) {
			db, p, w := recoveryFixture(t, kind)
			p.run = func(context.Context) error { panic("private provider credential") }
			// A panic must become a persisted failure, not escape the native boundary.
			func() {
				defer func() {
					if recover() != nil {
						t.Fatal("native panic escaped")
					}
				}()
				if err := w.Once(context.Background()); err != nil {
					t.Fatal(err)
				}
			}()
			if calls(p) != 1 || checkpointCount(t, db) != 0 {
				t.Fatal("native/checkpoint outcome mismatch")
			}
			var detail string
			var failures int
			var deferred bool
			if err := db.QueryRow("SELECT last_error,failures,next_retry_at>now() FROM worker_item_failures WHERE kind=$1 AND item_id=$2", kind, recoveryID).Scan(&detail, &failures, &deferred); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(detail, "task panic contained") || strings.Contains(detail, "private provider credential") || failures != 1 || !deferred {
				t.Fatal("panic was not durably and safely deferred")
			}
			// Observed native outcome remains unmodified; the same scan cannot replay it.
			if err := w.Once(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls(p) != 1 {
				t.Fatal("uncertain handler replayed without backoff")
			}
			if p.bypasses.Load() != 0 {
				t.Fatal("panic backoff reached installer bypass")
			}
			if kind == "operation" {
				var state string
				if err := db.QueryRow("SELECT state FROM operations WHERE id=$1", recoveryID).Scan(&state); err != nil || state != "running" {
					t.Fatal("panic invented terminal provider outcome")
				}
			}
		})
	}
}
