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
}

func (w Worker) Run(ctx context.Context) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.Once(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (w Worker) Once(ctx context.Context) error {
	w.Failures.ClearResolved(ctx)
	ops, err := w.Store.Operations(ctx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range ops {
		if !w.Failures.Due(ctx, "operation", x.ID) {
			continue
		}
		if err := w.Handler.RecoverOperation(ctx, x); err != nil {
			w.Failures.Fail(ctx, "operation", x.ID, x.AccountID, err)
			log.Printf("recovery operation %s account=%s: %v", x.ID, x.AccountID, err)
			continue
		}
		w.Failures.Clear(ctx, "operation", x.ID)
	}
	if w.Lifecycle != nil {
		_ = w.Lifecycle.Once(ctx)
	}
	deployments, err := w.Store.Deployments(ctx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range deployments {
		due := w.Failures.Due(ctx, "deployment", x.ID)
		if !due {
			if b, ok := w.Handler.(BackoffBypasser); !ok || !b.BypassDeploymentBackoff(ctx, x) {
				continue
			}
		}
		if err := w.Handler.RecoverDeployment(ctx, x); err != nil {
			w.Failures.Fail(ctx, "deployment", x.ID, x.AccountID, err)
			log.Printf("recovery deployment %s account=%s: %v", x.ID, x.AccountID, err)
			continue
		}
		w.Failures.Clear(ctx, "deployment", x.ID)
	}
	return nil
}
