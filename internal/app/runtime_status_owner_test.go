package app

import "testing"

func TestNetworkMaySetReadyOwnsOnlyNetworkStatuses(t *testing.T) {
	for _, s := range []string{"", "READY", "ISOLATION_WAIT", "PROVIDER_PROXY_ERROR", "PROVIDER_TRANSPORT_ERROR"} {
		if !networkMaySetReady(s) {
			t.Fatalf("network should own %q", s)
		}
	}
	for _, s := range []string{"ROTATION_BLOCKED_CAPACITY", "ROTATION_CAPACITY_BREAKING", "DELETE_PENDING", "DELETED", "PROVIDER_BLOCKED", "PROVIDER_PERMISSION_DENIED", "PROVIDER_TOKEN_INVALID"} {
		if networkMaySetReady(s) {
			t.Fatalf("network must not overwrite %q", s)
		}
	}
}
