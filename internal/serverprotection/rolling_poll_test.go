package serverprotection

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A slow enabled peer must not hold the next observation of healthy peers.
func TestProtectionHealthyPeerRenewsWhileEnabledPeerIsSlow(t *testing.T) {
	db := pollingDB(t)
	slow := pollingSeed(t, db, 1, true)
	for i := 2; i <= 6; i++ {
		pollingSeed(t, db, i, true)
	}
	c, ssh := newPollController(t, db)
	ssh.cleanup = make(chan struct{})
	ssh.blockPanel = slow
	budget := worker.NewWorkBudget(10)
	c.Admit = budget.Do
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("controller did not join canceled work")
		}
		if budget.Snapshot()["active"] != 0 || budget.Snapshot()["waiting"] != 0 {
			t.Error("admission leaked")
		}
		if db.Stats().InUse != 0 {
			t.Error("SQL session leaked")
		}
	}()
	waitPoll(t, 3*time.Second, func() bool {
		var n int
		return db.QueryRow("SELECT count(*) FROM server_protection_nodes WHERE state='APPLIED'").Scan(&n) == nil && n == 5
	})
	var previous time.Time
	if err := db.QueryRow("SELECT max(verified_at) FROM server_protection_nodes").Scan(&previous); err != nil {
		t.Fatal(err)
	}
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at=now() WHERE panel_id<>$1", slow)
	waitPoll(t, 6*time.Second, func() bool {
		var n int
		return db.QueryRow("SELECT count(*) FROM server_protection_nodes WHERE verified_at>$1", previous).Scan(&n) == nil && n == 5
	})
	ssh.mu.Lock()
	defer ssh.mu.Unlock()
	if ssh.active[true] != 1 || ssh.peak[true] > 6 {
		t.Fatalf("slow work not isolated: active=%d peak=%d", ssh.active[true], ssh.peak[true])
	}
}

// Waiting for the shared parent admission may not reserve a SQL connection,
// mutate scheduling evidence, or survive shutdown.
func TestProtectionRollingCancellationBeforeAdmission(t *testing.T) {
	db := pollingDB(t)
	pollingSeed(t, db, 1, true)
	c, ssh := newPollController(t, db)
	budget := worker.NewWorkBudget(1)
	c.Admit = budget.Do
	held, release := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = budget.Do(context.Background(), func(context.Context) error { close(held); <-release; return nil })
	}()
	<-held
	defer func() { close(release); wg.Wait() }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	waitPoll(t, 2*time.Second, func() bool { return budget.Snapshot()["waiting"] == 1 })
	if db.Stats().InUse != 0 {
		t.Fatal("SQL acquired before shared admission")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("blocked admission not joined")
	}
	if budget.Snapshot()["active"] != 1 || budget.Snapshot()["waiting"] != 0 {
		t.Fatal("wrong shared slot released")
	}
	var checked bool
	if err := db.QueryRow("SELECT checked_at IS NOT NULL FROM server_protection_nodes").Scan(&checked); err != nil || checked {
		t.Fatal("unadmitted work changed receipt", err)
	}
	ssh.mu.Lock()
	defer ssh.mu.Unlock()
	if len(ssh.calls) != 0 || ssh.uploads != 0 {
		t.Fatal("unadmitted work touched remote server")
	}
}

func TestProtectionContendedOwnerDoesNotSpinOrStarvePeers(t *testing.T) {
	db := pollingDB(t)
	locked := pollingSeed(t, db, 1, true)
	healthy := pollingSeed(t, db, 2, true)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock(hashtextextended($1,0))", "server-protection:"+locked); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1,0))", "server-protection:"+locked)
	c, _ := newPollController(t, db)
	budget := worker.NewWorkBudget(10)
	var calls atomic.Int64
	c.Admit = func(ctx context.Context, fn func(context.Context) error) error {
		calls.Add(1)
		return budget.Do(ctx, fn)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("contended controller leaked")
		}
	}()
	waitPoll(t, 2*time.Second, func() bool {
		var ok bool
		return db.QueryRow("SELECT state='APPLIED' FROM server_protection_nodes WHERE panel_id=$1", healthy).Scan(&ok) == nil && ok
	})
	time.Sleep(300 * time.Millisecond)
	if n := calls.Load(); n > 10 {
		t.Fatalf("contended owner caused tight retry loop: %d admissions", n)
	}
}

func TestProtectionRollingCoversBeyondPageAndRenews(t *testing.T) {
	db := pollingDB(t)
	slow := pollingSeed(t, db, 1, true)
	for i := 2; i <= 129; i++ {
		pollingSeed(t, db, i, true)
	}
	c, ssh := newPollController(t, db)
	ssh.cleanup = make(chan struct{})
	ssh.blockPanel = slow
	budget := worker.NewWorkBudget(10)
	c.Admit = budget.Do
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("fleet controller leaked")
		}
	}()
	waitPoll(t, 10*time.Second, func() bool {
		var n int
		return db.QueryRow("SELECT count(*) FROM server_protection_nodes WHERE state='APPLIED'").Scan(&n) == nil && n == 128
	})
	var previous time.Time
	if err := db.QueryRow("SELECT max(verified_at) FROM server_protection_nodes").Scan(&previous); err != nil {
		t.Fatal(err)
	}
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at=now() WHERE panel_id<>$1", slow)
	waitPoll(t, 10*time.Second, func() bool {
		var n int
		return db.QueryRow("SELECT count(*) FROM server_protection_nodes WHERE verified_at>$1", previous).Scan(&n) == nil && n == 128
	})
	ssh.mu.Lock()
	defer ssh.mu.Unlock()
	if ssh.active[true] != 1 || ssh.peak[true] > 6 {
		t.Fatal("rolling lane bound broken", ssh.active, ssh.peak)
	}
}

func TestProtectionDiscoveryDeadlineBeginsAfterAdmission(t *testing.T) {
	db := pollingDB(t)
	pollingSeed(t, db, 1, true)
	c, _ := newPollController(t, db)
	entered := make(chan struct{})
	var calls atomic.Int32
	c.Admit = func(ctx context.Context, fn func(context.Context) error) error {
		if calls.Add(1) == 1 {
			close(entered)
			// Simulate a shared permit becoming available after the query's 5s budget.
			select {
			case <-time.After(5500 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return fn(ctx)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	defer func() { cancel(); <-done }()
	<-entered
	waitPoll(t, 7*time.Second, func() bool {
		var n int
		return db.QueryRow("SELECT count(*) FROM server_protection_nodes WHERE state='APPLIED'").Scan(&n) == nil && n == 1
	})
}
