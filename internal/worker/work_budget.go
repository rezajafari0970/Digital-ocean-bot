package worker

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sync/atomic"
)

// WorkBudget admits complete top-level work BEFORE any transaction/session
// lease is taken. It is never acquired recursively. Waiting holds no SQL
// connection, and cancellation does not release an executing handler's slot.
type WorkBudget struct {
	slots   chan struct{}
	waiting atomic.Int64
}

func NewWorkBudget(limit int) *WorkBudget {
	if limit < 1 {
		panic("invalid work budget")
	}
	return &WorkBudget{slots: make(chan struct{}, limit)}
}
func (b *WorkBudget) Do(ctx context.Context, fn func(context.Context) error) error {
	endWait := supervision.Waiting(ctx)
	defer endWait()
	b.waiting.Add(1)
	select {
	case b.slots <- struct{}{}:
		b.waiting.Add(-1)
	case <-ctx.Done():
		b.waiting.Add(-1)
		return ctx.Err()
	}
	endWait()
	defer func() { <-b.slots }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return supervision.Work(ctx, fn)
}
func (b *WorkBudget) Snapshot() map[string]int64 {
	return map[string]int64{"limit": int64(cap(b.slots)), "active": int64(len(b.slots)), "waiting": b.waiting.Load()}
}

type AdmittedRecovery struct {
	Handler Handler
	Budget  *WorkBudget
}

func (a AdmittedRecovery) RecoverOperation(ctx context.Context, x RecoveryItem) error {
	return a.Budget.Do(ctx, func(ctx context.Context) error { return a.Handler.RecoverOperation(ctx, x) })
}
func (a AdmittedRecovery) RecoverDeployment(ctx context.Context, x RecoveryItem) error {
	return a.Budget.Do(ctx, func(ctx context.Context) error { return a.Handler.RecoverDeployment(ctx, x) })
}
func (a AdmittedRecovery) BypassDeploymentBackoff(ctx context.Context, x RecoveryItem) bool {
	h, ok := a.Handler.(BackoffBypasser)
	return ok && h.BypassDeploymentBackoff(ctx, x)
}

type AdmittedLifecycle struct {
	Handler LifecycleHandler
	Budget  *WorkBudget
}

func (a AdmittedLifecycle) ProcessLifecycle(ctx context.Context, x droplets.LifecycleItem) error {
	return a.Budget.Do(ctx, func(ctx context.Context) error { return a.Handler.ProcessLifecycle(ctx, x) })
}
