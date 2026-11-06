package app

import (
	"context"
	"sync"
	"time"
)

// Concurrent page assets share one fresh in-flight identity observation. There
// is no TTL cache: the next round after completion always re-verifies the route.
// Each caller may cancel independently; the browser lifetime bounds verification.
func coalesceBrowserRouteChecks(parent context.Context, verify func(context.Context) error) func(context.Context) error {
	type result struct {
		done chan struct{}
		err  error
	}
	var mu sync.Mutex
	var pending *result
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		mu.Lock()
		current := pending
		if current == nil {
			current = &result{done: make(chan struct{})}
			pending = current
			go func(r *result) {
				check, cancel := context.WithTimeout(parent, 8*time.Second)
				err := verify(check)
				cancel()
				mu.Lock()
				r.err = err
				pending = nil
				close(r.done)
				mu.Unlock()
			}(current)
		}
		mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-current.done:
			return current.err
		}
	}
}
