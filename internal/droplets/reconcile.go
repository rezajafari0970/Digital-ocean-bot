package droplets

import (
	"context"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

var ErrOutcomeStillUnknown = errors.New("provider outcome still unknown")

type Reconciler struct {
	Operations jobs.Store
	Provider   providers.ComputeDriver
}

func (r Reconciler) VerifyCreate(ctx context.Context, op jobs.Operation) (jobs.Operation, error) {
	if op.State != jobs.OperationVerifying && op.State != jobs.OperationUnknown {
		return op, nil
	}
	items, err := r.Provider.ListServers(ctx)
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
func (r Reconciler) AdoptUnknownCreate(ctx context.Context, op jobs.Operation, identityTag, name, region string) (jobs.Operation, error) {
	if op.State != jobs.OperationUnknown || op.ResourceID != "" {
		return op, nil
	}
	var items []providers.Server
	var err error
	if identityTag != "" {
		items, err = r.Provider.FindServerByIdentity(ctx, identityTag)
	} else {
		items, err = r.Provider.ListServers(ctx)
	}
	if err != nil {
		return op, err
	}
	var match *providers.Server
	for i := range items {
		x := &items[i]
		if (identityTag != "" || x.Name == name) && (region == "" || x.RegionID == region) {
			if match != nil {
				return op, ErrOutcomeStillUnknown
			}
			match = x
		}
	}
	if match == nil {
		return op, ErrOutcomeStillUnknown
	}
	op.ResourceID = match.ID
	op.State = jobs.OperationVerifying
	return op, r.Operations.Update(ctx, op)
}
func BuildDeleteOperation(accountID, providerID string) jobs.Operation {
	return jobs.Operation{AccountID: accountID, Kind: "DELETE_DROPLET", IdempotencyKey: "delete:" + accountID + ":" + providerID, ResourceID: providerID, State: jobs.OperationPlanned}
}
