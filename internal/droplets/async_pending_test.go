package droplets

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
)

type acceptedAsyncCompute struct {
	computeStub
	result  providers.CreateServerResult
	failure error
}

func (p *acceptedAsyncCompute) CreateServer(context.Context, providers.CreateServerRequest) (providers.CreateServerResult, error) {
	p.creates++
	return p.result, p.failure
}
func TestAcceptedAsyncCreateDoesNotMarkAccountTransportError(t *testing.T) {
	for _, tc := range []struct {
		name, job    string
		class        providers.ErrorClass
		wantCallback bool
	}{
		{"accepted_wait", "native-job", providers.ErrorAmbiguousOutcome, false},
		{"lost_response", "", providers.ErrorAmbiguousOutcome, true},
		{"transport", "native-job", providers.ErrorTransport, true},
		{"authentication", "native-job", providers.ErrorAuthentication, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &opStore{}
			p := &acceptedAsyncCompute{result: providers.CreateServerResult{OperationID: tc.job, Outcome: providers.OutcomeAmbiguous}, failure: &providers.Error{Class: tc.class, Operation: "create_server"}}
			called, success := false, false
			e := Executor{Operations: store, Provider: p, Gate: gateStub{}, OnCreateError: func(context.Context, error) { called = true }, OnCreateSuccess: func(context.Context, providers.CreateServerResult) { success = true }}
			profile := Profile{Name: "owned", Region: "US-SC", Size: "s", Image: "i", IdentityTag: "owned"}
			op, err := e.Create(context.Background(), BuildCreateOperation("account", profile), profile)
			if err == nil || called != tc.wantCallback || success || op.ProviderActionID != tc.job || p.creates != 1 {
				t.Fatalf("error=%v callback=%v success=%v job=%s posts=%d", err, called, success, op.ProviderActionID, p.creates)
			}
			if tc.class == providers.ErrorAmbiguousOutcome && store.saved.State != jobs.OperationUnknown {
				t.Fatal("pending job lost its durable reservation")
			}
		})
	}
}

func TestUndispatchedRegionalWaitDoesNotPoisonAccountHealth(t *testing.T) {
	for _, undispatched := range []bool{true, false} {
		store := &opStore{}
		failure := &providers.Error{Class: providers.ErrorRegionCapacity, Code: "REGION_CREATE_AWAITING_RECONCILIATION", Cause: providers.ErrRegionalCreateUnresolved}
		p := &acceptedAsyncCompute{result: providers.CreateServerResult{Outcome: providers.OutcomeRejected, ComputeNotDispatched: undispatched}, failure: failure}
		called, success := false, false
		e := Executor{Operations: store, Provider: p, Gate: gateStub{}, OnCreateError: func(context.Context, error) { called = true }, OnCreateSuccess: func(context.Context, providers.CreateServerResult) { success = true }}
		profile := Profile{Name: "owned", Region: "US-MI2", Size: "s", Image: "i", IdentityTag: "owned"}
		op, err := e.Create(context.Background(), BuildCreateOperation("account", profile), profile)
		if err == nil || called == undispatched || success || op.State != jobs.OperationFailed || op.ResourceID != "" || op.ErrorCode != string(providers.ErrorRegionCapacity) {
			t.Fatalf("unexpected local wait handling: undispatched=%v callback=%v success=%v op=%+v err=%v", undispatched, called, success, op, err)
		}
	}
}
