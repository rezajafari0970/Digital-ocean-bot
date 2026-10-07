package worker

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisionCancelsAndBoundsIgnoredHandlerWithoutReplay(t *testing.T) {
	r := supervision.New(nil)
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	var m Modules
	m.Supervisor = r
	m.Policies = map[string]supervision.Policy{"stuck": {Loop: 10 * time.Millisecond, Work: 10 * time.Millisecond}, "peer": {Loop: time.Minute, Work: time.Minute}}
	m.Grace = 20 * time.Millisecond
	m.Add(RoleControl, "stuck", func(ctx context.Context) {
		_ = supervision.Work(ctx, func(ctx context.Context) error { calls.Add(1); close(entered); <-release; return nil })
	})
	peerCanceled := make(chan struct{})
	m.Add(RoleControl, "peer", func(ctx context.Context) { <-ctx.Done(); close(peerCanceled) })
	done := make(chan error, 1)
	go func() { done <- m.Run(context.Background(), RoleControl) }()
	<-entered
	select {
	case e := <-done:
		if !errors.Is(e, supervision.ErrStalled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog blocked on ignored cancellation")
	}
	select {
	case <-peerCanceled:
	case <-time.After(time.Second):
		t.Fatal("role shutdown not requested")
	}
	if calls.Load() != 1 {
		t.Fatal("in-process replay")
	}
	close(release)
}
func TestAdmissionWaitIsSupervisedSeparately(t *testing.T) {
	r := supervision.New(nil)
	ctx, e := r.Register(context.Background(), "budget", supervision.Policy{Loop: time.Second, Work: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	b := NewWorkBudget(1)
	entered, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() { first <- b.Do(ctx, func(context.Context) error { close(entered); <-release; return nil }) }()
	<-entered
	c, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if e = b.Do(c, func(context.Context) error { t.Error("queued call executed"); return nil }); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	s := r.Snapshot().Modules[0]
	if s.Active != 1 || s.Waiting != 0 {
		t.Fatal(s)
	}
	close(release)
	if e = <-first; e != nil {
		t.Fatal(e)
	}
	if b.Snapshot()["active"] != 0 {
		t.Fatal("slot leaked")
	}
}

func TestSupervisionWithholdsReadyUntilEveryModuleProgresses(t *testing.T) {
	for _, mode := range []string{"return", "stall"} {
		t.Run(mode, func(t *testing.T) {
			var ready atomic.Bool
			m := Modules{Supervisor: supervision.New(nil), Policies: map[string]supervision.Policy{"startup": {Loop: 20 * time.Millisecond, Work: time.Second}}, Grace: 20 * time.Millisecond, Ready: func() error { ready.Store(true); return nil }}
			m.Add(RoleControl, "startup", func(ctx context.Context) {
				if mode == "stall" {
					<-ctx.Done()
				}
			})
			if err := m.Run(context.Background(), RoleControl); err == nil {
				t.Fatal("startup failure hidden")
			}
			if ready.Load() {
				t.Fatal("unobserved module published READY")
			}
		})
	}
}
func TestSupervisionSQLStallPersistsBeforeRestart(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	w.Admit = NewWorkBudget(1).Do
	p.run = func(ctx context.Context) error { _, err := db.ExecContext(ctx, "SELECT pg_sleep(60)"); return err }
	m := Modules{Supervisor: supervision.New(nil), Policies: map[string]supervision.Policy{"recovery": {Loop: 2 * time.Second, Work: 50 * time.Millisecond}}, Grace: 3 * time.Second}
	m.Add(RoleControl, "recovery", func(ctx context.Context) { _ = w.Run(ctx) })
	start := time.Now()
	if err := m.Run(context.Background(), RoleControl); !errors.Is(err, supervision.ErrStalled) {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second || calls(p) != 1 || checkpointCount(t, db) != 0 {
		t.Fatal("unbounded or incomplete cancellation")
	}
	var next time.Time
	if err := db.QueryRow("SELECT next_retry_at FROM worker_item_failures WHERE item_id=$1", recoveryID).Scan(&next); err != nil || time.Until(next) <= 0 {
		t.Fatal("backoff missing", err)
	}
	fresh := w
	p2 := &recoveryProbe{}
	fresh.Handler = p2
	if err := fresh.Once(context.Background()); err != nil || calls(p2) != 0 {
		t.Fatal("premature replay", err)
	}
}
func TestSupervisionOwnershipLossCancelsActiveDurableWork(t *testing.T) {
	db, p, w := recoveryFixture(t, "operation")
	lease, err := AcquireRole(context.Background(), db, RoleControl)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	entered := make(chan struct{})
	p.run = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	failure := make(chan error, 1)
	m := Modules{Supervisor: supervision.New(nil), Policies: map[string]supervision.Policy{"recovery": {Loop: time.Minute, Work: time.Minute}}, Grace: 3 * time.Second, Failure: failure}
	m.Add(RoleControl, "recovery", func(ctx context.Context) { _ = w.Run(ctx) })
	watch, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { failure <- lease.Run(watch) }()
	done := make(chan error, 1)
	go func() { done <- m.Run(context.Background(), RoleControl) }()
	<-entered
	if _, err = db.Exec("SELECT pg_terminate_backend($1)", lease.backend); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("ownership loss hidden")
		}
	case <-time.After(7 * time.Second):
		t.Fatal("ownership shutdown hung")
	}
	if calls(p) != 1 || checkpointCount(t, db) != 0 {
		t.Fatal("completion/replay")
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM worker_item_failures WHERE item_id=$1", recoveryID).Scan(&n); err != nil || n != 1 {
		t.Fatal("durable cancellation lost", err)
	}
}
