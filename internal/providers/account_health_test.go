package providers

import "testing"

func TestAccountConditionPreservesUnknownRestrictions(t *testing.T) {
	full := Capacity{LimitKnown: true, ComputeLimit: 3, ComputeInUse: 3}
	message := "Your team has created the maximum allowed number of Droplets. Please resolve this on the control panel."
	for _, tc := range []struct {
		provider, status, message string
		cap                       Capacity
		want                      string
	}{
		{"digitalocean", "warning", message, full, "CAPACITY_FULL"},
		{"vultr", "warning", message, full, "PROVIDER_WARNING"},
		{"upcloud", "warning", message, full, "PROVIDER_WARNING"},
		{"digitalocean", "warning", message + " Billing blocked.", full, "PROVIDER_WARNING"},
		{"digitalocean", "warning", message, Capacity{ComputeLimit: 3, ComputeInUse: 3}, "PROVIDER_WARNING"},
		{"digitalocean", "warning", message, Capacity{LimitKnown: true, ComputeLimit: 3, ComputeInUse: 2}, "PROVIDER_WARNING"},
		{"digitalocean", "locked", message, full, "PROVIDER_RESTRICTED"},
		{"upcloud", "trial_restricted", "", full, "TRIAL_RESTRICTED"},
		{"digitalocean", "active", "", full, "CAPACITY_FULL"},
		{"vultr", "active", "", full, "CAPACITY_FULL"},
		{"upcloud", "active", "", full, "CAPACITY_FULL"},
		{"vultr", "active", "", Capacity{}, "ACTIVE"},
	} {
		if got := AccountCondition(tc.provider, Account{Status: tc.status, StatusMessage: tc.message}, tc.cap); got != tc.want {
			t.Fatalf("%+v got %s", tc, got)
		}
	}
}
