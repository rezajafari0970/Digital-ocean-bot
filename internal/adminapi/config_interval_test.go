package adminapi

import "testing"

func TestCreationIntervalBoundaries(t *testing.T) {
	for _, seconds := range []int{-1, 0, 1, 360, 86400, 86401} {
		request := globalConfigRequest{Ports: []int{443}, TargetUsersPerInbound: 1, UsersPerSecond: 100, SNISelectionMode: "scored", CreationIntervalSeconds: &seconds}
		field, _ := validateGlobalConfig(request)
		invalid := seconds < 0 || seconds > 86400
		if invalid != (field == "creation_interval_seconds") {
			t.Fatalf("interval %d: field %q", seconds, field)
		}
	}
}
