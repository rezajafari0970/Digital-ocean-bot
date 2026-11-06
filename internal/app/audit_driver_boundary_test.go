package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"testing"
)

type auditRuntimeFactory struct {
	needs   providers.RuntimeRequirements
	opened  int
	request providers.OpenRequest
}

func (*auditRuntimeFactory) Name() string { return "extension" }
func (*auditRuntimeFactory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "extension"}
}
func (f *auditRuntimeFactory) RuntimeRequirements() providers.RuntimeRequirements { return f.needs }
func (f *auditRuntimeFactory) Open(_ context.Context, r providers.OpenRequest) (providers.Driver, error) {
	f.opened++
	f.request = r
	return auditRuntimeDriver{}, nil
}

type auditRuntimeDriver struct{}

func (auditRuntimeDriver) Name() string                         { return "extension" }
func (auditRuntimeDriver) Capabilities() providers.Capabilities { return providers.Capabilities{} }
func (auditRuntimeDriver) Health(context.Context) error         { return nil }

func TestAuditDriverRuntimeUsesPluginContract(t *testing.T) {
	f := &auditRuntimeFactory{}
	registry := providers.NewRegistry()
	if err := registry.Register(f); err != nil {
		t.Fatal(err)
	}
	c := Container{Providers: registry}
	cfg := AccountConfig{ID: "account", Provider: "extension", Network: network.Profile{Mode: network.RouteDirect}}
	d, err := c.openDriver(context.Background(), cfg, &http.Client{})
	if err != nil || d == nil || f.opened != 1 || f.request.AccountID != "account" || f.request.HTTPClient == nil {
		t.Fatal("registered extension required a provider-name branch", err)
	}
	for _, client := range []*http.Client{nil, {}} {
		cfg.Network.Mode = network.RouteProxyRequired
		if d, err = c.openDriver(context.Background(), cfg, client); !errors.Is(err, ErrNetworkNotReady) || d != nil || f.opened != 1 {
			t.Fatal("proxy-required runtime accepted implicit default transport")
		}
	}
	needs := &auditRuntimeFactory{needs: providers.RuntimeRequirements{ExternalIdentity: true}}
	r := providers.NewRegistry()
	if err := r.Register(needs); err != nil {
		t.Fatal(err)
	}
	c.Providers = r
	cfg.Network.Mode = network.RouteDirect
	if d, err = c.openDriver(context.Background(), cfg, &http.Client{}); err == nil || d != nil || needs.opened != 0 {
		t.Fatal("missing plugin dependency was not rejected before factory open")
	}
}
