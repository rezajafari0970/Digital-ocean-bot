package droplets

import (
	"context"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
)

func TestUnknownExistingCreateNeverIssuesSecondMutation(t *testing.T) {
	store := &opStore{exists: true, saved: jobs.Operation{ID: "op", AccountID: "a", Kind: "CREATE_DROPLET", IdempotencyKey: "deploy:x:create", State: jobs.OperationUnknown}}
	p := &computeStub{}
	e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
	_, err := e.Create(context.Background(), store.saved, Profile{Name: "n", Region: "fra1", Size: "s", Image: "i"})
	if !errors.Is(err, ErrOutcomeStillUnknown) {
		t.Fatalf("err=%v", err)
	}
	if p.creates != 0 {
		t.Fatalf("unsafe second create count=%d", p.creates)
	}
}

func TestDeleteSuccessRequiresLaterConfirmation(t *testing.T) {
	store := &opStore{}
	p := &computeStub{}
	e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
	op := BuildDeleteOperation("a", "42")
	got, err := e.Delete(context.Background(), op, "42")
	if err != nil {
		t.Fatal(err)
	}
	if p.deletes != 1 {
		t.Fatalf("deletes=%d", p.deletes)
	}
	if got.State != jobs.OperationVerifying {
		t.Fatalf("delete must verify absence, state=%s", got.State)
	}
	if got.State == jobs.OperationSucceeded {
		t.Fatal("provider 204 must not immediately finalize local deletion")
	}
}

func TestDeleteErrorIsAmbiguousUnknown(t *testing.T) {
	store := &opStore{}
	p := &computeStub{deleteErr: errors.New("transport reset")}
	e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
	op := BuildDeleteOperation("a", "42")
	got, err := e.Delete(context.Background(), op, "42")
	if err == nil {
		t.Fatal("expected provider error")
	}
	if got.State != jobs.OperationUnknown {
		t.Fatalf("state=%s", got.State)
	}
}
