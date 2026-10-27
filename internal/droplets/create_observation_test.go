package droplets

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
)

type observationStore struct {
	opStore
	missing  bool
	reserves int
}

func (s *observationStore) Reserve(ctx context.Context, op jobs.Operation) (jobs.Operation, bool, error) {
	s.reserves++
	return s.opStore.Reserve(ctx, op)
}
func (s *observationStore) Get(ctx context.Context, a, k string) (jobs.Operation, error) {
	if s.missing {
		return jobs.Operation{}, jobs.ErrOperationNotFound
	}
	return s.opStore.Get(ctx, a, k)
}
func TestCreateObservationCannotDispatchOrCrossOwnership(t *testing.T) {
	for _, name := range []string{"missing", "planned", "unknown", "running", "other_account", "other_key", "other_kind"} {
		t.Run(name, func(t *testing.T) {
			profile := Profile{Name: "owned-name", Region: "r", IdentityTag: "owned-tag"}
			want := BuildCreateOperation("account", profile)
			want.ID = "operation"
			saved := want
			saved.State = jobs.OperationUnknown
			saved.Attempt = 1
			store := &observationStore{opStore: opStore{saved: saved, exists: true}}
			switch name {
			case "missing":
				store.missing = true
			case "planned":
				store.saved.State = jobs.OperationPlanned
				store.saved.Attempt = 0
			case "running":
				store.saved.State = jobs.OperationRunning
			case "other_account":
				store.saved.AccountID = "another"
			case "other_key":
				store.saved.IdempotencyKey = "another"
			case "other_kind":
				store.saved.Kind = "DELETE_DROPLET"
			}
			provider := &computeStub{servers: []providers.Server{taggedServer("verified-id", "owned-name", "r", "owned-tag")}}
			// Mutation denial does not turn read-only reconciliation into dispatch.
			e := Executor{Operations: store, Provider: provider, Gate: gateStub{err: errors.New("new builds held")}}
			got, err := e.Create(WithCreateObservationOnly(context.Background()), want, profile)
			if provider.creates != 0 || store.reserves != 0 {
				t.Fatal("uncharged reconciliation dispatched or reserved a create")
			}
			switch name {
			case "missing", "planned":
				if !errors.Is(err, ErrCreateDispatchDeferred) {
					t.Fatalf("expected deferred: %v", err)
				}
			case "unknown":
				if err != nil || got.ResourceID != "verified-id" || got.State != jobs.OperationVerifying {
					t.Fatalf("did not adopt identity: %+v %v", got, err)
				}
			case "running":
				if err != nil || got.ResourceID != "" {
					t.Fatalf("running result changed: %+v %v", got, err)
				}
			default:
				if !errors.Is(err, jobs.ErrOperationConflict) {
					t.Fatalf("ownership mismatch accepted: %v", err)
				}
			}
		})
	}
}
