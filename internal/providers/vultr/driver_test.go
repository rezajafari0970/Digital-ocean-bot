package vultr

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
)

func TestFactoryMetadata(t *testing.T) {
	m := (Factory{}).Metadata()
	if m.Name != "vultr" || m.DisplayName != "Vultr" || m.CredentialLabel != "API key" {
		t.Fatalf("%+v", m)
	}
	if m.Defaults.Images.Family != "ubuntu" || len(m.Defaults.Images.Versions) != 3 || m.Defaults.LifetimeMinMinutes != 90 || m.Defaults.LifetimeMaxMinutes != 120 || m.Defaults.DesiredServers != 5 || m.Defaults.MaxConcurrent != 1 || !m.Defaults.FallbackAnyRegion {
		t.Fatalf("%+v", m.Defaults)
	}
}
func TestErrorTaxonomy(t *testing.T) {
	cases := []struct {
		status int
		want   providers.ErrorClass
	}{{401, providers.ErrorAuthentication}, {403, providers.ErrorPermissionDenied}, {404, providers.ErrorNotFound}, {409, providers.ErrorInvalidRequest}, {422, providers.ErrorInvalidRequest}, {429, providers.ErrorRateLimited}, {500, providers.ErrorTransport}, {502, providers.ErrorTransport}, {503, providers.ErrorTransport}}
	for _, tc := range cases {
		e := normalizeError("x", HTTPError{Status: tc.status, Message: "x"})
		var pe *providers.Error
		if !errors.As(e, &pe) || pe.Class != tc.want {
			t.Fatalf("status=%d err=%v", tc.status, e)
		}
	}
}

func TestMaintenance502IsRetryableTransportNotCapacity(t *testing.T) {
	err := normalizeError("create_server", HTTPError{Status: 502, Message: "We are currently conducting some software upgrades. Check back in a few minutes!"})
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Class != providers.ErrorTransport || pe.StatusCode != 502 {
		t.Fatalf("err=%v", err)
	}
	if providers.IsClass(err, providers.ErrorCapacity) || providers.IsClass(err, providers.ErrorRegionCapacity) {
		t.Fatalf("maintenance 502 must never be capacity evidence: %v", err)
	}
}
