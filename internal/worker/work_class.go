package worker

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sync/atomic"
)

// WorkClass caps one category before it can queue for the shared SQL budget.
// Waiting for a class permit holds neither a shared slot nor a SQL connection.
// All callers acquire class first, then shared; handlers never reacquire either.
type WorkClass struct {
	parent  *WorkBudget
	slots   chan struct{}
	waiting atomic.Int64
}

func NewWorkClass(parent *WorkBudget, limit int) *WorkClass {
	if parent == nil || limit < 1 || limit > cap(parent.slots) {
		panic("invalid work class budget")
	}
	return &WorkClass{parent: parent, slots: make(chan struct{}, limit)}
}
func (b *WorkClass) Do(ctx context.Context, fn func(context.Context) error) error {
	end := supervision.Waiting(ctx)
	b.waiting.Add(1)
	select {
	case b.slots <- struct{}{}:
		b.waiting.Add(-1)
	case <-ctx.Done():
		b.waiting.Add(-1)
		end()
		return ctx.Err()
	}
	end()
	defer func() { <-b.slots }()
	return b.parent.Do(ctx, fn)
}
func (b *WorkClass) Snapshot() map[string]int64 {
	return map[string]int64{"limit": int64(cap(b.slots)), "admitted": int64(len(b.slots)), "waiting": b.waiting.Load()}
}
