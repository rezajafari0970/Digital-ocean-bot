package scheduler

import "testing"

func TestProviderGateNegativeStates(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		status  string
		want    bool
	}{
		{"ready", true, "READY", true},
		{"disabled_panel", false, "READY", false},
		{"provider_disabled", true, "PROVIDER_BLOCKED", false},
		{"invalid_or_revoked_token", true, "PROVIDER_UNAVAILABLE", false},
		{"provider_unavailable", true, "PROVIDER_UNAVAILABLE", false},
		{"droplet_limit_reached", true, "PROVIDER_BLOCKED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerAllowsCreate(tc.enabled, tc.status); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
