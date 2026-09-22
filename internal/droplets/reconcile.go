package droplets

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"strconv"
)

var ErrOutcomeStillUnknown = errors.New("provider outcome still unknown")

type LookupProvider interface {
	ListDroplets(context.Context) ([]digitalocean.Resource, error)
}

type Reconciler struct {
	Operations jobs.Store
	Provider   LookupProvider
}

func (r Reconciler) VerifyCreate(ctx context.Context, op jobs.Operation) (jobs.Operation, error) {
	if op.State != jobs.OperationVerifying && op.State != jobs.OperationUnknown {
		return op, nil
	}
	items, err := r.Provider.ListDroplets(ctx)
	if err != nil {
		return op, err
	}
	if op.ResourceID != "" {
		for _, item := range items {
			if item.ID == op.ResourceID {
				op.State = jobs.OperationSucceeded
				return op, r.Operations.Update(ctx, op)
			}
		}
	}
	return op, ErrOutcomeStillUnknown
}

func BuildDeleteOperation(accountID string, providerID int) jobs.Operation {
	return jobs.Operation{AccountID: accountID, Kind: "DELETE_DROPLET", IdempotencyKey: "delete:" + accountID + ":" + strconv.Itoa(providerID), State: jobs.OperationPlanned}
}
