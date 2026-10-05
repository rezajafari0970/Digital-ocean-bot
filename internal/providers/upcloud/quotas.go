package upcloud

import (
	"bytes"
	"encoding/json"
	"sort"
)

// UpCloud's accountResourceLimits schema permits null only for these two Dev
// quotas. Preserve their presence and unknown value separately from known zero;
// a selected plan must never treat unknown quota as available capacity.
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
			switch key {
			case "cloud_server_dev_1xcpu_1gb_10gb_plans", "cloud_server_dev_1xcpu_1gb_plans":
				next[key] = nil
				continue
			default:
				issues = append(issues, quotaIssue(key, value))
				continue
			}
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
