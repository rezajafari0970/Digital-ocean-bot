package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"testing"
	"time"
)

func TestCloudflareManualLockWaitDoesNotReplay(t *testing.T) {
	db := adminTestDB(t)
	svc := cloudflare.Service{DB: db}
	ctx := context.Background()
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { done <- svc.Locked(ctx, "manual-fixture", func() error { close(held); <-release; return nil }) }()
	<-held
	timer := time.AfterFunc(75*time.Millisecond, func() { close(release) })
	defer timer.Stop()
	calls := 0
	e := svc.LockedWait(ctx, "manual-fixture", func() error { calls++; return errors.New("cloudflare_busy") })
	if e == nil || e.Error() != "cloudflare_busy" || calls != 1 {
		t.Fatal("manual function skipped or replayed", e, calls)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	// A canceled waiter must not enter the operation or retain a session lock.
	held, release, done = make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { done <- svc.Locked(ctx, "manual-fixture", func() error { close(held); <-release; return nil }) }()
	<-held
	timeout, cancel := context.WithTimeout(ctx, 75*time.Millisecond)
	e = svc.LockedWait(timeout, "manual-fixture", func() error { calls++; return nil })
	cancel()
	close(release)
	if holderErr := <-done; holderErr != nil {
		t.Fatal(holderErr)
	}
	if !errors.Is(e, context.DeadlineExceeded) || calls != 1 {
		t.Fatal("canceled lock waiter executed", e, calls)
	}
	if e = svc.Locked(ctx, "manual-fixture", func() error { return nil }); e != nil {
		t.Fatal("lock leaked", e)
	}
}
