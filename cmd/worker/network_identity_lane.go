package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sync"
	"time"
)

type identityScheduleOptions struct{ Deadline, Poll, Retry time.Duration }

var identityScheduleDefaults = identityScheduleOptions{45 * time.Second, time.Second, 10 * time.Second}

// Only two accounts can be assigned at once. A completion frees its lane
// immediately; a slow peer never delays the next account's renewal. Discovery
// closes its rows before admission is acquired again for an account. No SQL or
// account ownership is held while waiting, and operation time starts afterwards.
func runNetworkIdentities(parent context.Context,
	admit func(context.Context, func(context.Context) error) error,
	discover func(context.Context) ([]string, error),
	maintain func(context.Context, string) error, o identityScheduleOptions) {
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	done := make(chan string, 2)
	active := map[string]bool{}
	retry := map[string]time.Time{}
	timer := time.NewTimer(0)
	defer timer.Stop()
	reset := func(d time.Duration) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d)
	}
	var discoveryRetry time.Time
	for {
		supervision.Pulse(ctx)
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return
		case id := <-done:
			delete(active, id)
			retry[id] = time.Now().Add(o.Retry)
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		for id, at := range retry {
			if !now.Before(at) {
				delete(retry, id)
			}
		}
		if len(active) < 2 && !now.Before(discoveryRetry) {
			var ids []string
			err := admit(ctx, func(ctx context.Context) error {
				scan, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				var err error
				ids, err = discover(scan)
				return err
			})
			if err != nil {
				discoveryRetry = time.Now().Add(5 * o.Poll)
			} else {
				for _, id := range ids {
					if len(active) == 2 {
						break
					}
					if active[id] {
						continue
					}
					if at, ok := retry[id]; ok && time.Now().Before(at) {
						continue
					}
					active[id] = true
					wg.Add(1)
					go func(id string) {
						defer wg.Done()
						_ = admit(ctx, func(ctx context.Context) error {
							operation, cancel := context.WithTimeout(ctx, o.Deadline)
							defer cancel()
							if err := operation.Err(); err != nil {
								return err
							}
							return maintain(operation, id)
						})
						// Buffer equals max assignments: shutdown joins even without a receiver.
						done <- id
					}(id)
				}
			}
		}
		reset(o.Poll)
	}
}
