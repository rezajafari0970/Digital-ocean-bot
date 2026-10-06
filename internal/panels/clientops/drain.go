package clientops

import (
	"context"
	"time"
)

// Drain rechecks the existing atomic execution gate for every job. It removes
// the per-job idle tick without increasing global mutation concurrency.
func (e Executor) Drain(ctx context.Context) (int, error) {
	return drain(ctx, e.RunOne, 32, 5*time.Second)
}
func drain(ctx context.Context, one func(context.Context) (bool, error), limit int, budget time.Duration) (int, error) {
	started := time.Now()
	n := 0
	for n < limit && time.Since(started) < budget {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		ran, err := one(ctx)
		if ran {
			n++
		}
		if err != nil {
			return n, err
		}
		if !ran {
			break
		}
	}
	return n, nil
}
