package droplets

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
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
func (s *opStore) Update(_ context.Context, o jobs.Operation) error            { s.saved = o; return nil }

type providerStub struct{ creates int }

func (p *providerStub) CreateDroplet(context.Context, digitalocean.CreateDropletRequest) (digitalocean.Droplet, error) {
	p.creates++
	return digitalocean.Droplet{ID: 42, Status: "new"}, nil
}
func (p *providerStub) DeleteDroplet(context.Context, int) error { return nil }

func TestCreateIsIdempotent(t *testing.T) {
	store := &opStore{}
	provider := &providerStub{}
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
