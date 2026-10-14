package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"log"
)

var errProviderCredentialObsolete = fmt.Errorf("%w: %w", droplets.ErrMutationBlocked, ErrRuntimeGenerationObsolete)

// The timestamp belongs to the dispatched request, while the credential
// identity remains the one to which the constructed runtime was bound.
type providerRequestError struct {
	Cause   error
	Attempt ProviderAttempt
}

func (e *providerRequestError) Error() string { return e.Cause.Error() }
func (e *providerRequestError) Unwrap() error { return e.Cause }

type observedCreateDriver struct {
	providers.ComputeDriver
	Container Container
	Identity  ProviderAttempt
}

func (d observedCreateDriver) CreateServer(ctx context.Context, req providers.CreateServerRequest) (providers.CreateServerResult, error) {
	a, err := d.Container.BeginProviderAttempt(ctx, d.Identity.AccountID, false)
	if err != nil {
		return providers.CreateServerResult{Outcome: providers.OutcomeRejected}, fmt.Errorf("%w: credential binding unavailable: %v", droplets.ErrMutationBlocked, err)
	}
	if !sameProviderCredential(a, d.Identity) {
		return providers.CreateServerResult{Outcome: providers.OutcomeRejected}, errProviderCredentialObsolete
	}
	result, err := d.ComputeDriver.CreateServer(ctx, req)
	if err != nil {
		return result, &providerRequestError{Cause: err, Attempt: a}
	}
	return result, nil
}
func sameProviderCredential(a, b ProviderAttempt) bool {
	return a.AccountID == b.AccountID && a.SecretRef == b.SecretRef && a.CredentialAt.Valid == b.CredentialAt.Valid && (!a.CredentialAt.Valid || a.CredentialAt.Time.Equal(b.CredentialAt.Time))
}
func (c Container) recordBoundProviderError(ctx context.Context, cause error, proxyRequired bool) (bool, error) {
	var bound *providerRequestError
	if !errors.As(cause, &bound) {
		return false, nil
	}
	state := ClassifyAccountProviderError(cause, proxyRequired)
	return c.RecordProviderObservation(ctx, bound.Attempt, state, cause, cause.Error())
}
func (c Container) providerCreateErrorObserver(proxyRequired bool) func(context.Context, error) {
	return func(ctx context.Context, cause error) {
		applied, err := c.recordBoundProviderError(ctx, cause, proxyRequired)
		if err != nil {
			log.Printf("create provider observation persistence: %v", err)
		} else if !applied {
			log.Printf("create provider observation superseded or missing request provenance; coordinated refresh remains authoritative")
		}
	}
}

func (c Container) providerObservationExecutor(rt AccountRuntime, a ProviderAttempt, driver providers.ComputeDriver) droplets.Executor {
	return droplets.Executor{Operations: rt.Operations, Provider: observedCreateDriver{ComputeDriver: driver, Container: c, Identity: a}, Gate: rt.Gate, OnCreateError: c.providerCreateErrorObserver(rt.Config.Network.Mode == "proxy_required")}
}
