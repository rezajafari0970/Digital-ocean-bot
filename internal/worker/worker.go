package worker

import (
	"context"
	"log"
	"time"
)

type Handler interface {
	RecoverOperation(context.Context, RecoveryItem) error
	RecoverDeployment(context.Context, RecoveryItem) error
}
type BackoffBypasser interface {
	BypassDeploymentBackoff(context.Context, RecoveryItem) bool
}
type Worker struct {
	Store     RecoveryStore
	Handler   Handler
	Lifecycle *LifecycleWorker
	Failures  FailureStore
	Interval  time.Duration
	Batch     int
	Progress  func(time.Time)
}

func (w Worker) Run(ctx context.Context) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	dispatcher := NewDispatcher(6, 2)
	defer dispatcher.Wait()
	for {
		if err := w.round(ctx, func(key, account string, fn func()) { dispatcher.Submit(ctx, key, account, fn) }); err != nil {
			if ctx.Err() == nil {
				log.Printf("recovery discovery: %v", err)
			}
		} else if w.Progress != nil {
			w.Progress(time.Now())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (w Worker) Once(ctx context.Context) error {
	return w.round(ctx, func(_, _ string, fn func()) { fn() })
}
func (w Worker) round(ctx context.Context, submit func(string, string, func())) error {
	w.Failures.ClearResolved(ctx)
	ops, err := w.Store.Operations(ctx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range ops {
		x := x
		if !w.Failures.Due(ctx, "operation", x.ID) {
			continue
		}
		submit("operation:"+x.ID, x.AccountID, func() {
			if err := w.Handler.RecoverOperation(ctx, x); err != nil {
				w.Failures.Fail(ctx, "operation", x.ID, x.AccountID, err)
				log.Printf("recovery operation %s account=%s: %v", x.ID, x.AccountID, err)
				return
			}
			w.Failures.Clear(ctx, "operation", x.ID)
		})
	}
	if w.Lifecycle != nil {
		if err := w.Lifecycle.Once(ctx); err != nil {
			return err
		}
	}
	deployments, err := w.Store.Deployments(ctx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range deployments {
		x := x
		due := w.Failures.Due(ctx, "deployment", x.ID)
		if !due {
			if b, ok := w.Handler.(BackoffBypasser); !ok || !b.BypassDeploymentBackoff(ctx, x) {
				continue
			}
		}
		submit("deployment:"+x.ID, x.AccountID, func() {
			if err := w.Handler.RecoverDeployment(ctx, x); err != nil {
				w.Failures.Fail(ctx, "deployment", x.ID, x.AccountID, err)
				log.Printf("recovery deployment %s account=%s: %v", x.ID, x.AccountID, err)
				return
			}
			w.Failures.Clear(ctx, "deployment", x.ID)
		})
	}
	return nil
}
