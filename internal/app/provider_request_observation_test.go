package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"testing"
)

type requestRejectionCompute struct {
	providers.ComputeDriver
	failure        error
	creates, finds int
}

func (d *requestRejectionCompute) CreateServer(context.Context, providers.CreateServerRequest) (providers.CreateServerResult, error) {
	d.creates++
	return providers.CreateServerResult{Outcome: providers.OutcomeRejected}, d.failure
}
func (d *requestRejectionCompute) FindServerByIdentity(context.Context, string) ([]providers.Server, error) {
	d.finds++
	return nil, nil
}

func TestProviderObservationCreateDispatchOrderingAndRecovery(t *testing.T) {
	for _, class := range []providers.ErrorClass{providers.ErrorAccountLocked, providers.ErrorUnavailable, providers.ErrorAmbiguousOutcome} {
		t.Run(string(class), func(t *testing.T) {
			db := installerBootstrapDB(t)
			c := Container{DB: db}
			rt := observationFixture(t, db)
			ctx := context.Background()
			rt.Operations = jobs.SQLStore{DB: db}
			rt.Gate = network.AccountGate{Profile: rt.Config.Network}
			prepared := observationAttempt(t, c, rt)
			driver := &requestRejectionCompute{failure: &providers.Error{Class: class, StatusCode: 422, Message: "fixture provider response"}}
			executor := c.providerObservationExecutor(rt, prepared, driver)
			// Engine/executor exists before a newer successful coordinator observation.
			active := providers.Observation{Account: providers.Account{Status: "active"}}
			if ok, e := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), active, "READY", "newer refresh"); !ok || e != nil {
				t.Fatal(ok, e)
			}
			profile := droplets.Profile{Name: "fixture", Region: "fra1", Size: "s", Image: "ubuntu", IdentityTag: "observation-request-fixture"}
			op := droplets.BuildCreateOperation(rt.Config.ID, profile)
			result, cause := executor.Create(ctx, op, profile)
			if cause == nil || driver.creates != 1 {
				t.Fatal(result, cause)
			}
			var state string
			db.QueryRow(`SELECT provider_state FROM accounts WHERE id=$1`, rt.Config.ID).Scan(&state)
			if class == providers.ErrorAccountLocked {
				if state != "LOCKED" || result.State != jobs.OperationFailed {
					t.Fatal("fresh restriction suppressed", state, result)
				}
				// Recovery uses the request provenance, not its own later completion time.
				if ok, e := c.PublishProviderObservation(ctx, rt, observationAttempt(t, c, rt), active, "READY", "resolved"); !ok || e != nil {
					t.Fatal(ok, e)
				}
				if ok, e := c.recordBoundProviderError(ctx, cause, false); ok || e != nil {
					t.Fatal("recovery resurrected stale restriction", ok, e)
				}
			} else {
				if result.State != jobs.OperationUnknown || state != "ACTIVE" {
					t.Fatal("ambiguous create lost reconciliation", state, result)
				}
				_, _ = executor.Create(ctx, op, profile)
				if driver.creates != 1 || driver.finds == 0 {
					t.Fatal("unknown outcome replayed create", driver.creates, driver.finds)
				}
			}
		})
	}
}

func TestProviderObservationCredentialSourceRejectsChangedVersion(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	secret, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = secret.Put(ctx, rt.Config.ID, rt.Config.SecretRef, "fixture", []byte("fixture-old")); e != nil {
		t.Fatal(e)
	}
	source := accountCredentialSource{store: secret, accountID: rt.Config.ID, ref: rt.Config.SecretRef, db: db, expected: observationAttempt(t, c, rt)}
	v, e := source.Get(ctx)
	if e != nil || string(v) != "fixture-old" {
		t.Fatal("bound read failed", e)
	}
	wipe(v)
	if e = secret.Put(ctx, rt.Config.ID, rt.Config.SecretRef, "fixture", []byte("fixture-new")); e != nil {
		t.Fatal(e)
	}
	if v, e = source.Get(ctx); !errors.Is(e, ErrRuntimeGenerationObsolete) || len(v) != 0 {
		t.Fatal("stale runtime read replacement credential", e)
	}
}

func TestProviderObservationCredentialChangeBeforeDispatchIsRejected(t *testing.T) {
	db := installerBootstrapDB(t)
	c := Container{DB: db}
	rt := observationFixture(t, db)
	ctx := context.Background()
	rt.Operations = jobs.SQLStore{DB: db}
	rt.Gate = network.AccountGate{Profile: rt.Config.Network}
	driver := &requestRejectionCompute{failure: &providers.Error{Class: providers.ErrorAccountLocked}}
	executor := c.providerObservationExecutor(rt, observationAttempt(t, c, rt), driver)
	execBootstrap(t, db, `INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('fixture',$1,'fixture','x','x',1)`, rt.Config.ID)
	profile := droplets.Profile{Name: "fixture", Region: "fra1", Size: "s", Image: "ubuntu"}
	result, cause := executor.Create(ctx, droplets.BuildCreateOperation(rt.Config.ID, profile), profile)
	if !errors.Is(cause, droplets.ErrMutationBlocked) || result.State != jobs.OperationFailed || driver.creates != 0 {
		t.Fatal("known nondispatch became ambiguous or reached provider", result, cause, driver.creates)
	}
}
