package droplets

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"strconv"
)

var ErrLifecycleProviderID = errors.New("invalid lifecycle provider id")

type LifecycleExecutor interface {
	Delete(context.Context, jobs.Operation, int) (jobs.Operation, error)
}
type LifecycleEngine struct {
	Store    LifecycleStore
	Executor LifecycleExecutor
}

func (e LifecycleEngine) Process(ctx context.Context, item LifecycleItem) error {
	switch item.State {
	case Ready:
		ok, err := e.Store.Transition(ctx, item.ID, Ready, Expiring)
		if err != nil || !ok {
			return err
		}
		item.State = Expiring
		return e.Store.Event(ctx, item, Expiring)
	case Expiring:
		ok, err := e.Store.Transition(ctx, item.ID, Expiring, Retiring)
		if err != nil || !ok {
			return err
		}
		item.State = Retiring
		return e.Store.Event(ctx, item, Retiring)
	case Retiring:
		id, err := strconv.Atoi(item.ProviderID)
		if err != nil {
			return ErrLifecycleProviderID
		}
		op := BuildDeleteOperation(item.AccountID, id)
		op.IdempotencyKey = "lifecycle-delete:" + item.ID
		result, err := e.Executor.Delete(ctx, op, id)
		if err != nil {
			return err
		}
		if result.State != jobs.OperationVerifying && result.State != jobs.OperationSucceeded {
			return nil
		}
		ok, err := e.Store.Transition(ctx, item.ID, Retiring, Deleting)
		if err != nil || !ok {
			return err
		}
		item.State = Deleting
		return e.Store.Event(ctx, item, Deleting)
	case Deleting:
		return nil
	}
	return nil
}
