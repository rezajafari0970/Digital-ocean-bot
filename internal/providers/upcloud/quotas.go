package upcloud

import (
	"bytes"
	"encoding/json"
	"sort"
)

// Preserve null extension quotas without treating them as zero or absent.
// Known non-nullable quota fields remain strict. A selected plan must never
// treat a present unknown quota or usage value as available capacity.
type resourceLimits map[string]*number

func (limits *resourceLimits) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw == nil {
		*limits = nil
		return nil
	}
	next := make(resourceLimits, len(raw))
	var issues []string
	for key, value := range raw {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			known, nullable := resourceQuotaNullability(key)
			if !known || nullable {
				next[key] = nil
				continue
			}
			issues = append(issues, quotaIssue(key, value))
			continue
		}
		var n number
		if err := json.Unmarshal(value, &n); err != nil {
			issues = append(issues, quotaIssue(key, value))
			continue
		}
		next[key] = &n
	}
	if len(issues) > 0 {
		sort.Strings(issues)
		if len(issues) > 64 {
			issues = issues[:64]
		}
		return &responseError{Code: issues[0], QuotaIssues: issues}
	}
	*limits = next
	return nil
}

// This known schema is shared with diagnostic naming; unknown extension names
// remain private. Absence from the schema never authorizes resource capacity.
func resourceQuotaNullability(key string) (known, nullable bool) {
	switch key {
	case "cloud_server_dev_1xcpu_1gb_10gb_plans", "cloud_server_dev_1xcpu_1gb_plans":
		return true, true
	case "cores", "memory", "public_ipv4", "public_ipv6", "storage_total",
		"storage_maxiops", "storage_standard", "storage_hdd", "storage_ssd",
		"managed_databases", "network_peerings", "file_storages",
		"managed_container_registries", "managed_kubernetes", "tags", "networks",
		"network_gateways_essentials", "network_gateways", "load_balancers_essentials",
		"load_balancers", "gpus", "detached_interfaces", "detached_floating_ips",
		"managed_object_storages", "ntp_excess_gib", "routers":
		return true, false
	}
	return false, false
}
