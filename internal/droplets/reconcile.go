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
	ListDropletModels(context.Context) ([]digitalocean.Droplet, error)
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

func (r Reconciler) AdoptUnknownCreate(ctx context.Context, op jobs.Operation, identityTag, name, region string) (jobs.Operation, error) {
	if op.State != jobs.OperationUnknown || op.ResourceID != "" {
		return op, nil
	}
	items, err := r.Provider.ListDropletModels(ctx)
	if err != nil {
		return op, err
	}
	var match *digitalocean.Droplet
	for i := range items {
		x := &items[i]
		tagged := false
		for _, tag := range x.Tags {
			if identityTag != "" && tag == identityTag {
				tagged = true
				break
			}
		}
		if (tagged || (identityTag == "" && x.Name == name)) && (region == "" || x.Region.Slug == region) {
			if match != nil {
				return op, ErrOutcomeStillUnknown
			}
			match = x
		}
	}
	if match == nil {
		return op, ErrOutcomeStillUnknown
	}
	op.ResourceID = strconv.Itoa(match.ID)
	op.State = jobs.OperationVerifying
	return op, r.Operations.Update(ctx, op)
}

func BuildDeleteOperation(accountID string, providerID int) jobs.Operation {
	return jobs.Operation{AccountID: accountID, Kind: "DELETE_DROPLET", IdempotencyKey: "delete:" + accountID + ":" + strconv.Itoa(providerID), State: jobs.OperationPlanned}
}
