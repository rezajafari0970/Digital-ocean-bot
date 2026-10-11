package residentialsync

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"sync"
)

// Keep the process-wide number of native route reads bounded independently of
// fleet size. Drivers must explicitly opt in; SSH and custom drivers stay serial.
var routeProofSlots = make(chan struct{}, 8)

func runRouteProofChecks(ctx context.Context, exec sanaei.SessionExecutor, checks []func(context.Context) error) error {
	workers := 1
	if driver, ok := exec.(interface{ RouteProofConcurrency() int }); ok {
		workers = max(1, min(4, driver.RouteProofConcurrency()))
	}
	if workers == 1 {
		for _, check := range checks {
			if err := check(ctx); err != nil {
				return err
			}
		}
		return ctx.Err()
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan func(context.Context) error)
	var wg sync.WaitGroup
	var first sync.Once
	var failure error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for check := range jobs {
				select {
				case routeProofSlots <- struct{}{}:
				case <-work.Done():
					return
				}
				if work.Err() != nil {
					<-routeProofSlots
					return
				}
				err := check(work)
				<-routeProofSlots
				if err != nil {
					first.Do(func() { failure = err; cancel() })
					return
				}
			}
		}()
	}
send:
	for _, check := range checks {
		select {
		case jobs <- check:
		case <-work.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait() // No late native request may outlive the owner or its config lock.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return failure
}
