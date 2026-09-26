package worker

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"log"
	"time"
)

type LifecycleHandler interface {
	ProcessLifecycle(context.Context, droplets.LifecycleItem) error
}
type LifecycleWorker struct {
	Store    droplets.LifecycleStore
	Handler  LifecycleHandler
	Failures FailureStore
	Batch    int
}

func (w LifecycleWorker) Once(ctx context.Context) error {
	items, err := w.Store.Due(ctx, time.Now().UTC(), w.Batch)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !w.Failures.Due(ctx, "lifecycle", item.ID) {
			continue
		}
		if err := w.Handler.ProcessLifecycle(ctx, item); err != nil {
			w.Failures.Fail(ctx, "lifecycle", item.ID, item.AccountID, err)
			log.Printf("lifecycle item %s account=%s state=%s: %v", item.ID, item.AccountID, item.State, err)
			continue
		}
		w.Failures.Clear(ctx, "lifecycle", item.ID)
	}
	return nil
}
