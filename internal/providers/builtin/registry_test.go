package builtin

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/kamatera"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/linode"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
	"reflect"
	"testing"
)

type contractFactory struct{ providers.Driver }

func (f contractFactory) Metadata() providers.Metadata { return providers.Metadata{Name: f.Name()} }
func (f contractFactory) Open(context.Context, providers.OpenRequest) (providers.Driver, error) {
	return f.Driver, nil
}

func TestAuditBuiltinProviderContracts(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Names(), []string{"aws", "digitalocean", "kamatera", "linode", "ovh", "upcloud", "vultr"}) {
		t.Fatal("compiled provider set changed")
	}
	for _, name := range r.Names() {
		m, ok := r.Metadata(name)
		if !ok || m.Name != name || m.WebsiteURL == "" {
			t.Fatal("provider metadata incomplete")
		}
		needs, ok := r.Requirements(name)
		if !ok {
			t.Fatal("runtime contract missing")
		}
		want := providers.RuntimeRequirements{}
		switch name {
		case "aws", "ovh", "kamatera":
			want = providers.RuntimeRequirements{ExternalIdentity: true, Resources: true, Absence: true}
		case "upcloud":
			want = providers.RuntimeRequirements{Plans: true, Cleanup: true}
		}
		if needs != want {
			t.Fatalf("changed persisted-account dependency policy for %s", name)
		}
	}
	// Verify the actual driver method sets without credentials, network or cloud mutations.
	for _, driver := range []providers.Driver{&aws.Driver{}, &digitalocean.Driver{}, &kamatera.Driver{}, &linode.Driver{}, &ovh.Driver{}, &upcloud.Driver{}, &vultr.Driver{}} {
		check := providers.NewRegistry()
		if err := check.Register(contractFactory{driver}); err != nil {
			t.Fatal(err)
		}
		if _, err := check.Open(context.Background(), driver.Name(), providers.OpenRequest{}); err != nil {
			t.Fatalf("%s: %v", driver.Name(), err)
		}
	}
}
