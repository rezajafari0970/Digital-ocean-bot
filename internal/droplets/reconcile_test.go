package droplets

import (
	"context"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
)

type lookupStub struct{ droplets []digitalocean.Droplet }

func (p lookupStub) ListDroplets(context.Context) ([]digitalocean.Resource, error) { return nil, nil }
func (p lookupStub) ListDropletModels(context.Context) ([]digitalocean.Droplet, error) {
	return p.droplets, nil
}

func taggedDroplet(id int, name, region string, tags ...string) digitalocean.Droplet {
	return digitalocean.Droplet{ID: id, Name: name, Region: digitalocean.Region{Slug: region}, Tags: tags}
}

func TestAdoptUnknownCreateUsesUniqueTag(t *testing.T) {
	store := &opStore{exists: true, saved: jobs.Operation{ID: "op", AccountID: "a", State: jobs.OperationUnknown}}
	r := Reconciler{Operations: store, Provider: lookupStub{droplets: []digitalocean.Droplet{
		taggedDroplet(1, "same", "fra1", "managed-by-digital-ocean-bot", "other"),
		taggedDroplet(42, "same", "fra1", "managed-by-digital-ocean-bot", "dob-deployment-x"),
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
	r := Reconciler{Operations: store, Provider: lookupStub{droplets: []digitalocean.Droplet{
		taggedDroplet(1, "a", "fra1", "dob-deployment-x"),
		taggedDroplet(2, "b", "fra1", "dob-deployment-x"),
	}}}
	got, err := r.AdoptUnknownCreate(context.Background(), store.saved, "dob-deployment-x", "", "fra1")
	if err != ErrOutcomeStillUnknown {
		t.Fatalf("err=%v", err)
	}
	if got.ResourceID != "" || got.State != jobs.OperationUnknown {
		t.Fatalf("unsafe adoption: %+v", got)
	}
}
