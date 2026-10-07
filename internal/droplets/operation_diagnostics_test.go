package droplets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestOperationDiagnosticSecretSafety(t *testing.T) {
	for _, stage := range []operationStage{stagePreCreate, stageCreate, stageCreateRejected, stageCreateAmbiguous, stageCreateEgress, stageDeletePreEgress, stageDelete, stageDeleteEgress, 255} {
		for _, c := range []providers.ErrorClass{providers.ErrorCapacity, providers.ErrorAuthentication, providers.ErrorClass("SECRET_SENTINEL")} {
			for _, status := range []int{-1, 0, 403, 600, 1000000} {
				err := fmt.Errorf("SECRET_SENTINEL: %w", &providers.Error{Class: c, StatusCode: status, Code: "SECRET_SENTINEL", Operation: "SECRET_SENTINEL", Message: "SECRET_SENTINEL", Cause: errors.New("SECRET_SENTINEL")})
				code, msg := operationDiagnostic(stage, err)
				if code == "" || msg == "" || strings.Contains(code+msg, "SECRET_SENTINEL") || len(code) > 32 || len(msg) > 80 {
					t.Fatalf("unsafe diagnostic %q %q", code, msg)
				}
				if strings.Contains(msg, "HTTP") != (status >= 100 && status <= 599) {
					t.Fatalf("invalid status normalization %q", msg)
				}
			}
		}
	}
	for _, tc := range []struct {
		err  error
		code string
	}{
		{context.Canceled, "cancelled"}, {context.DeadlineExceeded, "deadline_exceeded"},
		{ErrMutationBlocked, "mutation_blocked"}, {ErrOutcomeStillUnknown, "ambiguous_outcome"},
		{errors.New("SECRET_SENTINEL"), "unknown"},
	} {
		code, _ := operationDiagnostic(stageCreate, fmt.Errorf("SECRET_SENTINEL: %w", tc.err))
		if code != tc.code {
			t.Fatalf("code %q want %q", code, tc.code)
		}
	}
}

type diagnosticFailStore struct {
	opStore
	failure error
}

func (s *diagnosticFailStore) Update(ctx context.Context, op *jobs.Operation) error {
	if op.State == jobs.OperationFailed || op.State == jobs.OperationUnknown {
		return s.failure
	}
	return s.opStore.Update(ctx, op)
}

func TestOperationDiagnosticsPersistenceFailureStopsProviderFallback(t *testing.T) {
	persistence := errors.New("journal unavailable")
	providerErr := &providers.Error{Class: providers.ErrorRegionCapacity, StatusCode: 503}
	store := &diagnosticFailStore{failure: persistence}
	p := &computeStub{createErr: providerErr}
	profile := Profile{Name: "fixture", Region: "fixture"}
	e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
	_, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if !errors.Is(err, persistence) || providers.IsClass(err, providers.ErrorRegionCapacity) || p.creates != 1 || store.saved.State != jobs.OperationRunning {
		t.Fatalf("err=%v calls=%d durable=%s", err, p.creates, store.saved.State)
	}
	// The unsuccessful persistence leaves a running reservation. It cannot be
	// replayed as a second provider mutation.
	_, err = e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
	if err != nil || p.creates != 1 {
		t.Fatalf("replayed after journal fault: calls=%d err=%v", p.creates, err)
	}
}

func TestOperationDiagnosticsBlockedAndUnknown(t *testing.T) {
	t.Run("precreate", func(t *testing.T) {
		store := &opStore{}
		p := &computeStub{}
		e := Executor{Operations: store, Provider: p, Gate: gateStub{}, PreCreateCheck: func(context.Context) error { return ErrMutationBlocked }}
		profile := Profile{Name: "fixture"}
		op, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
		if !errors.Is(err, ErrMutationBlocked) || p.creates != 0 || op.ErrorCode != "mutation_blocked" || store.saved.ErrorCode != op.ErrorCode {
			t.Fatalf("op=%+v err=%v calls=%d", op, err, p.creates)
		}
	})
	t.Run("transport", func(t *testing.T) {
		store := &opStore{}
		providerErr := &providers.Error{Class: providers.ErrorTransport}
		p := &computeStub{createErr: providerErr}
		e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
		profile := Profile{Name: "fixture"}
		op, err := e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
		if err != providerErr || op.State != jobs.OperationUnknown || op.ErrorCode != "transport" {
			t.Fatalf("op=%+v err=%v", op, err)
		}
		_, _ = e.Create(context.Background(), BuildCreateOperation("a", profile), profile)
		if p.creates != 1 {
			t.Fatal("unknown create replayed")
		}
	})
	t.Run("delete", func(t *testing.T) {
		store := &opStore{}
		providerErr := &providers.Error{Class: providers.ErrorTransport}
		p := &diagnosticDeleteProvider{failure: providerErr}
		e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
		op, err := e.Delete(context.Background(), BuildDeleteOperation("a", "fixture"), "fixture")
		if err != providerErr || op.State != jobs.OperationUnknown || op.ErrorCode != "transport" || p.calls != 1 {
			t.Fatalf("op=%+v err=%v calls=%d", op, err, p.calls)
		}
	})
}

type diagnosticDeleteProvider struct {
	computeStub
	failure error
	calls   int
}

func (p *diagnosticDeleteProvider) DeleteServer(context.Context, string) error {
	p.calls++
	return p.failure
}
