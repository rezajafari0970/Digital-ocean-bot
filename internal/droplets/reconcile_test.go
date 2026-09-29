package droplets

import (
	"context"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestAdoptUnknownCreateUsesUniqueTag(t *testing.T) {
	store := &opStore{exists: true, saved: jobs.Operation{ID: "op", AccountID: "a", State: jobs.OperationUnknown}}
	r := Reconciler{Operations: store, Provider: &computeStub{servers: []providers.Server{
		taggedServer("1", "same", "fra1", "managed-by-digital-ocean-bot", "other"),
		taggedServer("42", "same", "fra1", "managed-by-digital-ocean-bot", "dob-deployment-x"),
	}}}
	got, err := r.AdoptUnknownCreate(context.Background(), store.saved, "dob-deployment-x", "same", "fra1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ResourceID != "42" || got.State != jobs.OperationVerifying {
		t.Fatalf("unexpected adoption: %+v", got)
	}
}

func TestAdoptUnknownCreateAmbiguousTagStaysUnknown(t *testing.T) {
	store := &opStore{exists: true, saved: jobs.Operation{ID: "op", AccountID: "a", State: jobs.OperationUnknown}}
	r := Reconciler{Operations: store, Provider: &computeStub{servers: []providers.Server{
		taggedServer("1", "a", "fra1", "dob-deployment-x"),
		taggedServer("2", "b", "fra1", "dob-deployment-x"),
	}}}
	got, err := r.AdoptUnknownCreate(context.Background(), store.saved, "dob-deployment-x", "", "fra1")
	if err != ErrOutcomeStillUnknown {
		t.Fatalf("err=%v", err)
	}
	if got.ResourceID != "" || got.State != jobs.OperationUnknown {
		t.Fatalf("unsafe adoption: %+v", got)
	}
}
