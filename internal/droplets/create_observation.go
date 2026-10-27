package droplets

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
)

// Uncharged workflow reconciliation must never become another provider POST,
// even if a concurrent observer turns the prior intent into a definitive failure.
type createObservationOnlyKey struct{}

var ErrCreateDispatchDeferred = errors.New("create dispatch requires a budgeted workflow attempt")

func WithCreateObservationOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, createObservationOnlyKey{}, true)
}
func (e Executor) observeCreate(ctx context.Context, want jobs.Operation, profile Profile) (jobs.Operation, error) {
	if e.Operations == nil || e.Provider == nil {
		return want, ErrMutationBlocked
	}
	op, err := e.Operations.Get(ctx, want.AccountID, want.IdempotencyKey)
	if errors.Is(err, jobs.ErrOperationNotFound) {
		return want, ErrCreateDispatchDeferred
	}
	if err != nil {
		return want, err
	}
	if op.ID == "" || op.AccountID != want.AccountID || op.Kind != "CREATE_DROPLET" || op.Kind != want.Kind || op.IdempotencyKey != want.IdempotencyKey {
		return op, jobs.ErrOperationConflict
	}
	if op.State == jobs.OperationPlanned && op.Attempt == 0 {
		return op, ErrCreateDispatchDeferred
	}
	if op.Attempt < 1 {
		return op, jobs.ErrOperationConflict
	}
	if op.State == jobs.OperationUnknown && op.ResourceID == "" {
		return (Reconciler{Operations: e.Operations, Provider: e.Provider}).AdoptUnknownCreate(ctx, op, profile.IdentityTag, profile.Name, profile.Region)
	}
	return op, nil
}
