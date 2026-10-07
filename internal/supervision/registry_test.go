package supervision

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Registry, context.Context, *time.Time) {
	t.Helper()
	now := time.Unix(1000, 0)
	r := New(func() time.Time { return now })
	ctx, err := r.Register(context.Background(), "module", Policy{Loop: 10 * time.Second, Work: 5 * time.Second, Idle: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return r, ctx, &now
}
func TestHungWorkNotHiddenByLoopOrHeartbeat(t *testing.T) {
	r, ctx, now := fixture(t)
	_, done, e := Begin(ctx, "work", 0)
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	*now = now.Add(6 * time.Second)
	Pulse(ctx)
	Idle(ctx)
	_ = r.Snapshot()
	if !errors.Is(r.Check(), ErrStalled) || ctx.Err() == nil || r.Snapshot().Healthy {
		t.Fatal("hung task hidden")
	}
}
func TestDeclaredLongStageAndSiblingIndependence(t *testing.T) {
	r, ctx, now := fixture(t)
	root, finish, e := Begin(ctx, "work", 0)
	if e != nil {
		t.Fatal(e)
	}
	_, stageEnd, e := Begin(root, "execute", 15*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	*now = now.Add(10 * time.Minute)
	if e = r.Check(); e != nil {
		t.Fatal("valid installer stage canceled", e)
	}
	stageEnd()
	*now = now.Add(4 * time.Second)
	if e = r.Check(); e != nil {
		t.Fatal(e)
	}
	finish()
	// A separate task still has its own deadline despite another long phase.
	root, finish, e = Begin(ctx, "work", 0)
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
	_, stageEnd, e = Begin(root, "execute", 15*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	defer stageEnd()
	_, siblingEnd, e := Begin(ctx, "work", 0)
	if e != nil {
		t.Fatal(e)
	}
	defer siblingEnd()
	*now = now.Add(6 * time.Second)
	if r.Check() == nil {
		t.Fatal("sibling hidden by stage")
	}
}
func TestWaitingDoesNotSpendAncestorExecutionBudget(t *testing.T) {
	r, ctx, now := fixture(t)
	root, finish, e := Begin(ctx, "work", 0)
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
	waitEnd := Waiting(root)
	*now = now.Add(2 * time.Minute)
	if e = r.Check(); e != nil {
		t.Fatal("admission charged as execution", e)
	}
	waitEnd()
	waitEnd()
	*now = now.Add(4 * time.Second)
	if e = r.Check(); e != nil {
		t.Fatal(e)
	}
	*now = now.Add(2 * time.Second)
	if r.Check() == nil {
		t.Fatal("execution did not resume")
	}
}
func TestIdleDeadlineAndModuleIsolation(t *testing.T) {
	r, ctx, now := fixture(t)
	peer, e := r.Register(context.Background(), "peer", Policy{Loop: time.Hour, Work: time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	Idle(ctx)
	*now = now.Add(65 * time.Second)
	if r.Check() != nil {
		t.Fatal("intentional idle rejected")
	}
	*now = now.Add(6 * time.Second)
	if r.Check() == nil || ctx.Err() == nil || peer.Err() != nil {
		t.Fatal("wrong cancellation scope")
	}
}
func TestInvalidContractsAndRegressingClock(t *testing.T) {
	r, ctx, now := fixture(t)
	if _, e := r.Register(ctx, "module", Policy{Loop: time.Minute, Work: time.Minute}); e == nil {
		t.Fatal("duplicate module")
	}
	if _, _, e := Begin(ctx, "raw-secret-or-resource-id", time.Second); e == nil {
		t.Fatal("arbitrary label accepted")
	}
	if _, _, e := Begin(ctx, "work", 25*time.Hour); e == nil {
		t.Fatal("unbounded stage accepted")
	}
	if e := r.Check(); e != nil {
		t.Fatal(e)
	}
	*now = now.Add(-time.Second)
	if r.Check() == nil {
		t.Fatal("clock regression hidden")
	}
}
func TestUnbalancedStageLatchesFailure(t *testing.T) {
	r, ctx, _ := fixture(t)
	root, finish, _ := Begin(ctx, "work", 0)
	_, end, _ := Begin(root, "verify", time.Minute)
	finish()
	end()
	if r.Check() == nil || r.Snapshot().Healthy {
		t.Fatal("parent abandoned child")
	}
}
