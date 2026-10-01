package droplets

import (
	"context"
	"errors"
	"fmt"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

var ErrMutationBlocked = errors.New("droplet mutation blocked")

type MutationGate interface{ AllowMutation() error }

type Executor struct {
	Operations      jobs.Store
	Provider        providers.ComputeDriver
	Gate            MutationGate
	EgressCheck     func(context.Context) error
	PreCreateCheck  func(context.Context) error
	OnCreateError   func(context.Context, error)
	OnCreateSuccess func(context.Context, providers.CreateServerResult)
}

func (e Executor) Create(ctx context.Context, op jobs.Operation, profile Profile) (jobs.Operation, error) {
	if e.Gate == nil || e.Provider == nil {
		return op, ErrMutationBlocked
	}
	if err := e.Gate.AllowMutation(); err != nil {
		return op, fmt.Errorf("%w: %v", ErrMutationBlocked, err)
	}
	if e.EgressCheck != nil {
		if err := e.EgressCheck(ctx); err != nil {
			return op, err
		}
	}
	reserved, fresh, err := e.Operations.Reserve(ctx, op)
	if err != nil {
		return op, err
	}
	if !fresh {
		if reserved.State == jobs.OperationUnknown && reserved.ResourceID == "" {
			return (Reconciler{Operations: e.Operations, Provider: e.Provider}).AdoptUnknownCreate(ctx, reserved, profile.IdentityTag, profile.Name, profile.Region)
		}
		return reserved, nil
	}
	reserved.State = jobs.OperationRunning
	reserved.Attempt++
	if err := e.Operations.Update(ctx, reserved); err != nil {
		return reserved, err
	}
	if e.PreCreateCheck != nil {
		if err := e.PreCreateCheck(ctx); err != nil {
			reserved.State = jobs.OperationFailed
			_ = e.Operations.Update(ctx, reserved)
			return reserved, err
		}
	}
	ssh := []string(nil)
	if profile.SSHKeyID != "" {
		ssh = []string{profile.SSHKeyID}
	}
	result, err := e.Provider.CreateServer(ctx, providers.CreateServerRequest{Name: profile.Name, RegionID: profile.Region, PlanID: profile.Size, ImageID: profile.Image, SSHKeyRefs: ssh, Tags: []string{"managed-by-digital-ocean-bot"}, Identity: profile.IdentityTag})
	if err != nil {
		if e.OnCreateError != nil {
			e.OnCreateError(ctx, err)
		}
		class := providers.Class(err)
		if class == providers.ErrorAuthentication || class == providers.ErrorPermissionDenied || class == providers.ErrorAccountLocked || class == providers.ErrorCapacity || class == providers.ErrorRegionCapacity || class == providers.ErrorImageUnavailable || class == providers.ErrorInvalidRequest || class == providers.ErrorNotFound || errors.Is(err, ErrMutationBlocked) {
			reserved.State = jobs.OperationFailed
		} else {
			reserved.State = jobs.OperationUnknown
		}
		_ = e.Operations.Update(ctx, reserved)
		return reserved, err
	}
	if result.Outcome == providers.OutcomeRejected || result.ServerID == "" {
		reserved.State = jobs.OperationFailed
		_ = e.Operations.Update(ctx, reserved)
		return reserved, errors.New("provider rejected create without server id")
	}
	if result.Outcome == providers.OutcomeAmbiguous {
		reserved.State = jobs.OperationUnknown
		_ = e.Operations.Update(ctx, reserved)
		return reserved, ErrOutcomeStillUnknown
	}
	if e.OnCreateSuccess != nil {
		e.OnCreateSuccess(ctx, result)
	}
	reserved.ResourceID = result.ServerID
	reserved.State = jobs.OperationVerifying
	if err := e.Operations.Update(ctx, reserved); err != nil {
		return reserved, err
	}
	if e.EgressCheck != nil {
		if err := e.EgressCheck(ctx); err != nil {
			reserved.State = jobs.OperationUnknown
			_ = e.Operations.Update(ctx, reserved)
			return reserved, err
		}
	}
	return reserved, nil
}

func (e Executor) Delete(ctx context.Context, op jobs.Operation, providerID string) (jobs.Operation, error) {
	if e.Gate == nil || e.Provider == nil || e.Gate.AllowMutation() != nil {
		return op, ErrMutationBlocked
	}
	reserved, fresh, err := e.Operations.Reserve(ctx, op)
	if err != nil {
		return op, err
	}
	if !fresh {
		if reserved.State != jobs.OperationUnknown || reserved.ResourceID != providerID {
			return reserved, nil
		}
	} else {
		reserved.ResourceID = providerID
		if err := e.Operations.Update(ctx, reserved); err != nil {
			return reserved, err
		}
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
	if err := e.Provider.DeleteServer(ctx, providerID); err != nil {
		if providers.IsClass(err, providers.ErrorNotFound) {
			reserved.State = jobs.OperationVerifying
			return reserved, e.Operations.Update(ctx, reserved)
		}
		reserved.State = jobs.OperationUnknown
		_ = e.Operations.Update(ctx, reserved)
		return reserved, err
	}
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
