package worker

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sync"
	"sync/atomic"
)

// WorkBudget admits complete top-level work BEFORE any transaction/session
// lease is taken. It is never acquired recursively. Waiting holds no SQL
// connection, and cancellation does not release an executing handler's slot.
type WorkBudget struct {
	mu                             sync.Mutex
	limit, costLimit, active, cost int
	changed                        chan struct{}
	waiting                        atomic.Int64
}

func NewWorkBudget(limit int) *WorkBudget { return NewWeightedWorkBudget(limit, limit) }

// Costs bound retained SQL connections, separately from concurrent operations.
func NewWeightedWorkBudget(limit, costLimit int) *WorkBudget {
	if limit < 1 || costLimit < 1 {
		panic("invalid work budget")
	}
	return &WorkBudget{limit: limit, costLimit: costLimit, changed: make(chan struct{})}
}
func (b *WorkBudget) Do(ctx context.Context, fn func(context.Context) error) error {
	return b.do(ctx, 1, fn)
}

type weightedAdmission struct {
	budget *WorkBudget
	weight int
}

func (b *WorkBudget) WithWeight(weight int) WorkAdmission {
	if weight < 1 || weight > b.costLimit {
		panic("invalid work weight")
	}
	return weightedAdmission{b, weight}
}
func (a weightedAdmission) Capacity() int { return min(a.budget.limit, a.budget.costLimit/a.weight) }
func (a weightedAdmission) Do(ctx context.Context, fn func(context.Context) error) error {
	return a.budget.do(ctx, a.weight, fn)
}
func (b *WorkBudget) do(ctx context.Context, weight int, fn func(context.Context) error) error {
	endWait := supervision.Waiting(ctx)
	defer endWait()
	b.waiting.Add(1)
	for {
		b.mu.Lock()
		if err := ctx.Err(); err != nil {
			b.mu.Unlock()
			b.waiting.Add(-1)
			return err
		}
		if b.active < b.limit && b.cost+weight <= b.costLimit {
			b.active++
			b.cost += weight
			b.mu.Unlock()
			b.waiting.Add(-1)
			break
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			b.waiting.Add(-1)
			return ctx.Err()
		case <-changed:
		}
	}
	endWait()
	defer func() {
		b.mu.Lock()
		b.active--
		b.cost -= weight
		close(b.changed)
		b.changed = make(chan struct{})
		b.mu.Unlock()
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return supervision.Work(ctx, fn)
}
func (b *WorkBudget) Capacity() int { return b.limit }
func (b *WorkBudget) Snapshot() map[string]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]int64{"limit": int64(b.limit), "active": int64(b.active), "waiting": b.waiting.Load(), "cost_limit": int64(b.costLimit), "cost_active": int64(b.cost)}
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
