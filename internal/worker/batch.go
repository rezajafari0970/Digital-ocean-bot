package worker

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sync"
)

// RunBoundedBatch bounds goroutines as well as admission. The caller's root
// work node observes queue waiting; every callback, including runtime login,
// runs inside the shared admission/task contract. Cancellation always joins.
func RunBoundedBatch(ctx context.Context, count, concurrency int, admit func(context.Context, func(context.Context) error) error, fn func(context.Context, int) error) error {
	if count <= 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > count {
		concurrency = count
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var outcome error
	add := func(err error) {
		if err != nil {
			mu.Lock()
			outcome = errors.Join(outcome, err)
			mu.Unlock()
		}
	}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				endWait := supervision.Waiting(ctx)
				var index int
				var ok bool
				select {
				case <-ctx.Done():
					endWait()
					return
				case index, ok = <-jobs:
				}
				endWait()
				if !ok {
					return
				}
				run := func(c context.Context) error { return fn(c, index) }
				if admit != nil {
					add(admit(ctx, run))
				} else {
					add(supervision.Work(ctx, run))
				}
			}
		}()
	}
send:
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			break send
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	return errors.Join(outcome, ctx.Err())
}
