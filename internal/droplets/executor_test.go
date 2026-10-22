package droplets

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
)

type gateStub struct{ err error }

func (g gateStub) AllowMutation() error { return g.err }

type opStore struct {
	saved  jobs.Operation
	exists bool
}

func (s *opStore) Reserve(_ context.Context, o jobs.Operation) (jobs.Operation, bool, error) {
	if s.exists {
		return s.saved, false, nil
	}
	o.ID = "1"
	s.saved = o
	s.exists = true
	return o, true, nil
}
func (s *opStore) Get(context.Context, string, string) (jobs.Operation, error) { return s.saved, nil }
func (s *opStore) Update(_ context.Context, o *jobs.Operation) error {
	s.saved = *o
	o.LockVersion++
	return nil
}

func TestCreateErrorCallbackReceivesProviderCapacityError(t *testing.T) {
	store := &opStore{}
	capacityErr := &providers.Error{Class: providers.ErrorCapacity, Operation: "create_server", Message: "account saturated"}
	provider := &computeStub{createErr: capacityErr}
	var seen error
	e := Executor{Operations: store, Provider: provider, Gate: gateStub{}, OnCreateError: func(_ context.Context, err error) { seen = err }}
	profile := Profile{Name: "p", Region: "ewr", Size: "s", Image: "ubuntu"}
	_, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if err == nil || seen != capacityErr || store.saved.State != jobs.OperationFailed {
		t.Fatalf("err=%v seen=%v state=%s", err, seen, store.saved.State)
	}
}

func TestCreateSuccessCallbackRunsAfterAcceptedCreate(t *testing.T) {
	store := &opStore{}
	provider := &computeStub{}
	called := false
	e := Executor{Operations: store, Provider: provider, Gate: gateStub{}, OnCreateSuccess: func(_ context.Context, result providers.CreateServerResult) { called = result.ServerID == "42" }}
	profile := Profile{Name: "p", Region: "ewr", Size: "s", Image: "ubuntu"}
	got, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if err != nil || !called || got.ResourceID != "42" {
		t.Fatalf("got=%+v called=%v err=%v", got, called, err)
	}
}

func TestCreateIsIdempotent(t *testing.T) {
	store := &opStore{}
	provider := &computeStub{}
	e := Executor{Operations: store, Provider: provider, Gate: gateStub{}}
	profile := Profile{Name: "p", Region: "ams3", Size: "s", Image: "ubuntu"}
	op := BuildCreateOperation("a", profile)
	first, err := e.Create(context.Background(), op, profile)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != jobs.OperationVerifying || provider.creates != 1 {
		t.Fatal("first create failed")
	}
	_, err = e.Create(context.Background(), op, profile)
	if err != nil {
		t.Fatal(err)
	}
	if provider.creates != 1 {
		t.Fatal("duplicate provider mutation")
	}
}

func TestAmbiguousCreateDoesNotAdvanceCapacitySuccess(t *testing.T) {
	store := &opStore{}
	result := providers.CreateServerResult{ServerID: "maybe-42", Outcome: providers.OutcomeAmbiguous}
	provider := &computeStub{createResult: &result}
	successCalled := false
	e := Executor{
		Operations:      store,
		Provider:        provider,
		Gate:            gateStub{},
		OnCreateSuccess: func(context.Context, providers.CreateServerResult) { successCalled = true },
	}
	profile := Profile{Name: "p", Region: "ewr", Size: "s", Image: "ubuntu"}
	_, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if !errors.Is(err, ErrOutcomeStillUnknown) || successCalled || store.saved.State != jobs.OperationUnknown {
		t.Fatalf("err=%v successCalled=%v state=%s", err, successCalled, store.saved.State)
	}
}

func TestPreCreateCheckBlocksProviderMutation(t *testing.T) {
	store := &opStore{}
	provider := &computeStub{}
	e := Executor{
		Operations:     store,
		Provider:       provider,
		Gate:           gateStub{},
		PreCreateCheck: func(context.Context) error { return ErrMutationBlocked },
	}
	profile := Profile{Name: "p", Region: "ewr", Size: "s", Image: "ubuntu"}
	_, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if !errors.Is(err, ErrMutationBlocked) || provider.creates != 0 || store.saved.State != jobs.OperationFailed {
		t.Fatalf("err=%v creates=%d state=%s", err, provider.creates, store.saved.State)
	}
}

func TestCreateTransportErrorStaysUnknownForReconciliation(t *testing.T) {
	store := &opStore{}
	transportErr := &providers.Error{Class: providers.ErrorTransport, Operation: "create_server", Message: "timeout"}
	provider := &computeStub{createErr: transportErr}
	successCalled := false
	e := Executor{
		Operations:      store,
		Provider:        provider,
		Gate:            gateStub{},
		OnCreateSuccess: func(context.Context, providers.CreateServerResult) { successCalled = true },
	}
	profile := Profile{Name: "p", Region: "ewr", Size: "s", Image: "ubuntu"}
	_, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if !providers.IsClass(err, providers.ErrorTransport) || successCalled || store.saved.State != jobs.OperationUnknown {
		t.Fatalf("err=%v success=%v state=%s", err, successCalled, store.saved.State)
	}
}

func TestCreateResumesOnlyUndispatchedPlannedOperation(t *testing.T) {
	for _, state := range []jobs.OperationState{jobs.OperationPlanned, jobs.OperationRunning, jobs.OperationUnknown} {
		t.Run(string(state), func(t *testing.T) {
			p := Profile{Name: "fixed", Region: "r", Size: "p", Image: "i", IdentityTag: "identity"}
			op := BuildCreateOperation("a", p)
			op.State = state
			store := &opStore{saved: op, exists: true}
			driver := &computeStub{}
			e := Executor{Operations: store, Provider: driver, Gate: gateStub{}}
			_, _ = e.Create(context.Background(), op, p)
			expected := 0
			if state == jobs.OperationPlanned {
				expected = 1
			}
			if driver.creates != expected {
				t.Fatal("unsafe create resumption", state, driver.creates)
			}
		})
	}
}

type networkPreparationFailure struct {
	computeStub
	sent bool
}

func (d networkPreparationFailure) CreateServer(context.Context, providers.CreateServerRequest) (providers.CreateServerResult, error) {
	return providers.CreateServerResult{Outcome: providers.OutcomeRejected, ComputeNotDispatched: !d.sent}, &providers.Error{Class: providers.ErrorTransport, Operation: "network_setup"}
}
func TestNetworkPreparationFailureDoesNotStrandCompute(t *testing.T) {
	for _, sent := range []bool{false, true} {
		store := &opStore{}
		executor := Executor{Operations: store, Provider: &networkPreparationFailure{sent: sent}, Gate: gateStub{}}
		p := Profile{Name: "p", Region: "eu-west-1", Size: "t3.micro", Image: "ubuntu"}
		_, e := executor.Create(context.Background(), BuildCreateOperation("a", p), p)
		want := jobs.OperationFailed
		if sent {
			want = jobs.OperationUnknown
		}
		if e == nil || store.saved.State != want {
			t.Fatal(sent, store.saved.State, e)
		}
	}
}
