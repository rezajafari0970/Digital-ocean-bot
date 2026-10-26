package jobs

import "testing"

func TestQuotaReasonSurvivesPersistenceGrammar(t *testing.T) {
	for _, message := range []string{"create_server: region_capacity; reason=FLAVOR_QUOTA_EXCEEDED", "create_server: region_capacity (HTTP 409); reason=REGIONAL_QUOTA_EXCEEDED"} {
		code, got := SafeDiagnostic("region_capacity", message)
		if code != "region_capacity" || got != message {
			t.Fatal(code, got)
		}
	}
	for _, message := range []string{"create_server: region_capacity; reason=SECRET_SENTINEL", "create_server: region_capacity; reason=FLAVOR_QUOTA_EXCEEDED;secret", "SECRET_SENTINEL; reason=FLAVOR_QUOTA_EXCEEDED"} {
		_, got := SafeDiagnostic("region_capacity", message)
		if got != "operation: region_capacity" {
			t.Fatal(got)
		}
	}
}
