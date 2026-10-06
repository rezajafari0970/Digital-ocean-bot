package droplets

import (
	"context"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
)

var deleteFault = errors.New("injected storage interruption")

type deleteFaultStore struct {
	saved        jobs.Operation
	exists       bool
	failResource bool
	failState    jobs.OperationState
	updates      int
}

func (s *deleteFaultStore) Reserve(_ context.Context, o jobs.Operation) (jobs.Operation, bool, error) {
	if s.exists {
		return s.saved, false, nil
	}
	o.ID = "fixture"
	o.ResourceID = ""
	s.saved = o
	s.exists = true
	// SQL Reserve returns the input identity but persists it on the next Update.
	returned := o
	return returned, true, nil
}
func (s *deleteFaultStore) Get(context.Context, string, string) (jobs.Operation, error) {
	return s.saved, nil
}
func (s *deleteFaultStore) Update(_ context.Context, o *jobs.Operation) error {
	if s.failResource && o.State == jobs.OperationPlanned && o.ResourceID != "" {
		s.failResource = false
		return deleteFault
	}
	if s.failState != "" && o.State == s.failState {
		s.failState = ""
		return deleteFault
	}
	if s.saved.LockVersion != o.LockVersion {
		return jobs.ErrOperationVersionConflict
	}
	o.LockVersion++
	s.saved = *o
	s.updates++
	return nil
}
func deleteStored(state jobs.OperationState, resource string) *deleteFaultStore {
	o := BuildDeleteOperation("account", "server")
	o.ID = "fixture"
	o.State = state
	o.ResourceID = resource
	return &deleteFaultStore{saved: o, exists: true}
}
func TestDeleteResumePlannedAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name     string
		state    jobs.OperationState
		resource string
	}{
		{"reserved-only", jobs.OperationPlanned, ""},
		{"resource-persisted", jobs.OperationPlanned, "server"},
		{"unknown", jobs.OperationUnknown, "server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := deleteStored(tc.state, tc.resource)
			p := &computeStub{}
			e := Executor{Operations: s, Provider: p, Gate: gateStub{}}
			got, err := e.Delete(context.Background(), BuildDeleteOperation("account", "server"), "server")
			if err != nil || got.State != jobs.OperationVerifying || p.deletes != 1 || got.ResourceID != "server" {
				t.Fatalf("state=%s deletes=%d err=%v", got.State, p.deletes, err)
			}
			if _, err = e.Delete(context.Background(), BuildDeleteOperation("account", "server"), "server"); err != nil || p.deletes != 1 {
				t.Fatalf("replayed verifying delete: %d %v", p.deletes, err)
			}
		})
	}
}
func TestDeleteResumeFaultBeforeProvider(t *testing.T) {
	for _, stage := range []string{"resource-persist", "running-claim"} {
		t.Run(stage, func(t *testing.T) {
			s := &deleteFaultStore{failResource: stage == "resource-persist"}
			if stage == "running-claim" {
				s.failState = jobs.OperationRunning
			}
			p := &computeStub{}
			e := Executor{Operations: s, Provider: p, Gate: gateStub{}}
			op := BuildDeleteOperation("account", "server")
			if _, err := e.Delete(context.Background(), op, "server"); !errors.Is(err, deleteFault) || p.deletes != 0 || s.saved.State != jobs.OperationPlanned {
				t.Fatalf("unsafe interruption: %+v calls=%d err=%v", s.saved, p.deletes, err)
			}
			if _, err := e.Delete(context.Background(), op, "server"); err != nil || p.deletes != 1 || s.saved.State != jobs.OperationVerifying {
				t.Fatalf("resume failed calls=%d state=%s err=%v", p.deletes, s.saved.State, err)
			}
		})
	}
}
func TestDeleteResumeDeniedAndNetworkRecovery(t *testing.T) {
	s := deleteStored(jobs.OperationPlanned, "server")
	p := &computeStub{}
	e := Executor{Operations: s, Provider: p, Gate: gateStub{err: ErrMutationBlocked}}
	op := BuildDeleteOperation("account", "server")
	if _, err := e.Delete(context.Background(), op, "server"); !errors.Is(err, ErrMutationBlocked) || p.deletes != 0 || s.updates != 0 {
		t.Fatal("gate bypass", err)
	}
	e.Gate = gateStub{}
	e.EgressCheck = func(context.Context) error { return errors.New("network not ready") }
	if _, err := e.Delete(context.Background(), op, "server"); err == nil || p.deletes != 0 || s.saved.State != jobs.OperationUnknown {
		t.Fatal("egress bypass", err)
	}
	e.EgressCheck = func(context.Context) error { return nil }
	if _, err := e.Delete(context.Background(), op, "server"); err != nil || p.deletes != 1 {
		t.Fatal("network recovery failed", err)
	}
}
func TestDeleteResumeRejectsIdentityMismatch(t *testing.T) {
	for _, field := range []string{"account", "kind", "key", "resource", "unknown-empty"} {
		t.Run(field, func(t *testing.T) {
			s := deleteStored(jobs.OperationPlanned, "server")
			switch field {
			case "account":
				s.saved.AccountID = "other"
			case "kind":
				s.saved.Kind = "CREATE_DROPLET"
			case "key":
				s.saved.IdempotencyKey = "other"
			case "resource":
				s.saved.ResourceID = "other"
			case "unknown-empty":
				s.saved.State = jobs.OperationUnknown
				s.saved.ResourceID = ""
			}
			p := &computeStub{}
			e := Executor{Operations: s, Provider: p, Gate: gateStub{}}
			if _, err := e.Delete(context.Background(), BuildDeleteOperation("account", "server"), "server"); !errors.Is(err, jobs.ErrOperationConflict) || p.deletes != 0 || s.updates != 0 {
				t.Fatalf("identity changed calls=%d updates=%d err=%v", p.deletes, s.updates, err)
			}
		})
	}
}
func TestDeleteResumeNeverReplaysOtherStates(t *testing.T) {
	for _, state := range []jobs.OperationState{jobs.OperationRunning, jobs.OperationVerifying, jobs.OperationSucceeded, jobs.OperationFailed} {
		s := deleteStored(state, "server")
		p := &computeStub{}
		e := Executor{Operations: s, Provider: p, Gate: gateStub{}}
		if _, err := e.Delete(context.Background(), BuildDeleteOperation("account", "server"), "server"); err != nil || p.deletes != 0 || s.updates != 0 {
			t.Fatalf("replayed %s err=%v", state, err)
		}
	}
}
func TestDeleteResumeInterruptionAfterProviderDoesNotReplay(t *testing.T) {
	s := deleteStored(jobs.OperationPlanned, "server")
	s.failState = jobs.OperationVerifying
	p := &computeStub{}
	e := Executor{Operations: s, Provider: p, Gate: gateStub{}}
	op := BuildDeleteOperation("account", "server")
	if _, err := e.Delete(context.Background(), op, "server"); !errors.Is(err, deleteFault) || p.deletes != 1 || s.saved.State != jobs.OperationRunning {
		t.Fatal("bad post-provider failure", err)
	}
	if _, err := e.Delete(context.Background(), op, "server"); err != nil || p.deletes != 1 {
		t.Fatal("running must go to existing recovery", err)
	}
}

func TestDeleteResumeRejectsAttemptedPlannedOperation(t *testing.T) {
	s := deleteStored(jobs.OperationPlanned, "server")
	s.saved.Attempt = 1
	p := &computeStub{}
	e := Executor{Operations: s, Provider: p, Gate: gateStub{}}
	if _, err := e.Delete(context.Background(), BuildDeleteOperation("account", "server"), "server"); !errors.Is(err, jobs.ErrOperationConflict) || p.deletes != 0 || s.updates != 0 {
		t.Fatal("anomalous attempted plan replay", err)
	}
}
