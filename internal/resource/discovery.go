// Package resource contains process-local scheduling controls. It never owns
// provider work, SQL leases or persisted outcomes.
package resource

import (
	"context"
	"sync"
	"time"
)

// Discovery coalesces empty discovery across sibling consumers. A successful
// durable claim reopens parallel admission BEFORE executing potentially slow
// work. Existing work budgets, ownership fences and deadlines remain authoritative.
// Separate classes must use separate instances. The zero value is ready.
type Discovery struct {
	mu            sync.Mutex
	open, probing bool
	generation    uint64
	next          time.Time
}

// Do runs at most one probe per idle interval. Call found only after a durable
// claim succeeds. Skipped probes are ordinary idle work, never provider success.
// Callers keep their existing periodic wakeup, heartbeat and cancellation path.
func (g *Discovery) Do(ctx context.Context, now time.Time, interval time.Duration, run func(context.Context, func()) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if interval <= 0 {
		interval = time.Second
	}
	g.mu.Lock()
	if !g.open && (g.probing || now.Before(g.next)) {
		g.mu.Unlock()
		return nil
	}
	generation := g.generation
	if !g.open {
		g.probing = true
	}
	g.mu.Unlock()
	claimed, finished := false, false
	defer func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		finished = true
		if !claimed && generation == g.generation {
			g.open, g.probing = false, false
			g.next = now.Add(interval)
		}
	}()
	found := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if claimed || finished {
			return
		}
		claimed = true
		g.generation++
		g.open, g.probing = true, false
		g.next = time.Time{}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return run(ctx, found)
}
