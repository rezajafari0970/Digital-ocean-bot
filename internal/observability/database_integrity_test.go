package observability

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIntegrityReportCannotHideNewFailure(t *testing.T) {
	now := time.Now()
	current := integrityWatermark{Database: "test", OID: "42", Failures: 2920, LastFailure: 100, StatsReset: 10}
	base := integrityReport{Version: 1, Status: "ok", CheckedAt: float64(now.Unix()), Watermark: current}
	raw, _ := json.Marshal(base)
	if got := validateIntegrityReport(raw, current, now); got != "" {
		t.Fatal(got)
	}
	for _, field := range []string{"count", "timestamp", "reset", "identity"} {
		c := current
		switch field {
		case "count":
			c.Failures++
		case "timestamp":
			c.LastFailure++
		case "reset":
			c.StatsReset++
		case "identity":
			c.OID = "43"
		}
		if got := validateIntegrityReport(raw, c, now); got != "database_recovery_required" {
			t.Fatalf("%s: %s", field, got)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*integrityReport)
		want   string
	}{
		{"failed", func(r *integrityReport) { r.Status = "failed" }, "database_recovery_required"},
		{"stale", func(r *integrityReport) { r.CheckedAt = float64(now.Add(-9 * time.Hour).Unix()) }, "database_integrity_check_stale"},
		{"future", func(r *integrityReport) { r.CheckedAt = float64(now.Add(time.Hour).Unix()) }, "database_integrity_check_stale"},
		{"version", func(r *integrityReport) { r.Version = 2 }, "database_integrity_unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.change(&r)
			b, _ := json.Marshal(r)
			if got := validateIntegrityReport(b, current, now); got != tc.want {
				t.Fatal(got)
			}
		})
	}
	if got := validateIntegrityReport([]byte("{"), current, now); got != "database_integrity_unverified" {
		t.Fatal(got)
	}
}
