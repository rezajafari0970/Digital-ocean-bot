package usercapacity

import "testing"

func TestClientMutationLifecycleAllowed(t *testing.T) {
	for _, state := range []string{"READY", "EXPIRING"} {
		if !clientMutationLifecycleAllowed(state) {
			t.Fatalf("%s must allow client mutation", state)
		}
	}
	for _, state := range []string{"RETIRING", "DELETING", "DELETED", "PROVISIONING", ""} {
		if clientMutationLifecycleAllowed(state) {
			t.Fatalf("%s must not allow client mutation", state)
		}
	}
}
