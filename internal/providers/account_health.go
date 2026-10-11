package providers

import "strings"

// AccountCondition is display/diagnostic classification, never create
// authorization. A full allowance is distinct from auth/billing restrictions.
// Unknown warnings remain warnings even when capacity is also full.
func AccountCondition(provider string, a Account, c Capacity) string {
	status := strings.ToLower(strings.TrimSpace(a.Status))
	full := c.LimitKnown && c.ComputeLimit > 0 && c.ComputeInUse >= c.ComputeLimit
	if status == "warning" {
		message := strings.Join(strings.Fields(a.StatusMessage), " ")
		if provider == "digitalocean" && full && message == "Your team has created the maximum allowed number of Droplets. Please resolve this on the control panel." {
			return "CAPACITY_FULL"
		}
		return "PROVIDER_WARNING"
	}
	if status == "active" {
		if full {
			return "CAPACITY_FULL"
		}
		return "ACTIVE"
	}
	if status == "trial_restricted" {
		return "TRIAL_RESTRICTED"
	}
	return "PROVIDER_RESTRICTED"
}
