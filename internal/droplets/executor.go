package droplets

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
)

var ErrMutationBlocked = errors.New("droplet mutation blocked")

type MutationGate interface{ AllowMutation() error }
type Provider interface {
	CreateDroplet(context.Context, digitalocean.CreateDropletRequest) (digitalocean.Droplet, error)
	DeleteDroplet(context.Context, int) error
}

type Executor struct {
	Operations  jobs.Store
	Provider    Provider
	Gate        MutationGate
	EgressCheck func(context.Context) error
}

func (e Executor) Create(ctx context.Context, op jobs.Operation, profile Profile) (jobs.Operation, error) {
	if e.Gate == nil {
		return op, ErrMutationBlocked
	}
	if err := e.Gate.AllowMutation(); err != nil {
		return op, fmt.Errorf("%w: %v", ErrMutationBlocked, err)
	}
	reserved, fresh, err := e.Operations.Reserve(ctx, op)
	if err != nil {
		return op, err
	}
	if !fresh {
		if reserved.State == jobs.OperationUnknown && reserved.ResourceID == "" {
			if lookup, ok := e.Provider.(LookupProvider); ok {
				return (Reconciler{Operations: e.Operations, Provider: lookup}).AdoptUnknownCreate(ctx, reserved, profile.IdentityTag, profile.Name, profile.Region)
			}
		}
		return reserved, nil
	}
	if e.EgressCheck != nil {
		if err := e.EgressCheck(ctx); err != nil {
			reserved.State = jobs.OperationUnknown
			_ = e.Operations.Update(ctx, reserved)
			return reserved, err
		}
	}
	reserved.State = jobs.OperationRunning
	reserved.Attempt++
	if err := e.Operations.Update(ctx, reserved); err != nil {
		return reserved, err
	}
	d, err := e.Provider.CreateDroplet(ctx, digitalocean.CreateDropletRequest{Name: profile.Name, Region: profile.Region, Size: profile.Size, Image: profile.Image, SSHKeys: func() []any {
		if profile.SSHKeyID > 0 {
			return []any{profile.SSHKeyID}
		}
		return nil
	}(), Tags: []string{"managed-by-digital-ocean-bot", profile.IdentityTag}})
	if err != nil {
		// A deterministic provider rejection means no resource was created.
		// Only ambiguous/retryable outcomes are eligible for reconciliation.
		class := digitalocean.ClassifyError(err)
		if class == resilience.Permanent || errors.Is(err, ErrMutationBlocked) {
			reserved.State = jobs.OperationFailed
		} else {
			reserved.State = jobs.OperationUnknown
		}
		_ = e.Operations.Update(ctx, reserved)
		return reserved, err
	}
	reserved.ResourceID = strconv.Itoa(d.ID)
	if e.EgressCheck != nil {
		if err := e.EgressCheck(ctx); err != nil {
			reserved.State = jobs.OperationUnknown
			_ = e.Operations.Update(ctx, reserved)
			return reserved, err
		}
	}
	reserved.State = jobs.OperationVerifying
	return reserved, e.Operations.Update(ctx, reserved)
}

func (e Executor) Delete(ctx context.Context, op jobs.Operation, providerID int) (jobs.Operation, error) {
	if e.Gate == nil || e.Gate.AllowMutation() != nil {
		return op, ErrMutationBlocked
	}
	reserved, fresh, err := e.Operations.Reserve(ctx, op)
	if err != nil {
		return op, err
	}
	if !fresh {
		return reserved, nil
	}
	if e.EgressCheck != nil {
		if err := e.EgressCheck(ctx); err != nil {
			reserved.State = jobs.OperationUnknown
			_ = e.Operations.Update(ctx, reserved)
			return reserved, err
		}
	}
	reserved.State = jobs.OperationRunning
	reserved.Attempt++
	if err := e.Operations.Update(ctx, reserved); err != nil {
		return reserved, err
	}
	if err := e.Provider.DeleteDroplet(ctx, providerID); err != nil {
		reserved.State = jobs.OperationUnknown
		_ = e.Operations.Update(ctx, reserved)
		return reserved, err
	}
	reserved.ResourceID = strconv.Itoa(providerID)
	if e.EgressCheck != nil {
		if err := e.EgressCheck(ctx); err != nil {
			reserved.State = jobs.OperationUnknown
			_ = e.Operations.Update(ctx, reserved)
			return reserved, err
		}
	}
	reserved.State = jobs.OperationVerifying
	return reserved, e.Operations.Update(ctx, reserved)
}
