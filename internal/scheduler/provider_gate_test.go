package scheduler

import "testing"

func TestProviderGateNegativeStates(t *testing.T) {
	cases := []struct {
		name                        string
		enabled                     bool
		runtime, state, providerErr string
		want                        bool
	}{
		{"ready", true, "READY", "ACTIVE", "", true},
		{"locked", true, "READY", "LOCKED", "", false},
		{"transport", true, "READY", "ACTIVE", "TRANSPORT_ERROR", false},
		{"proxy", true, "READY", "ACTIVE", "PROXY_ERROR", false},
		{"runtime_blocked", true, "PROVIDER_BLOCKED", "ACTIVE", "", false},
		{"disabled", false, "READY", "ACTIVE", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerAllowsCreate(tc.enabled, tc.runtime, tc.state, tc.providerErr); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
