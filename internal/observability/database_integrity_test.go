package observability

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIntegrityReportCannotHideNewFailure(t *testing.T) {
	now := time.Now()
	current := integrityWatermark{Database: "test", OID: "42", Failures: 2920, LastFailure: 100, StatsReset: 10}
	base := integrityReport{Version: 1, Status: "ok", Checksums: "on", CheckedAt: float64(now.Unix()), Watermark: current}
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

func TestIntegrityReportCannotHideRecoveryOrExpiredBackup(t *testing.T) {
	now := time.Unix(1800000000, 0)
	current := integrityWatermark{Database: "fixture", OID: "42"}
	base := map[string]any{
		"version": 1, "status": "ok", "checksums": "on",
		"checked_at": now.Unix(), "watermark": current,
		"recovery_required": false,
	}
	cases := []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{"healthy", nil, ""},
		{"recovery_latched", map[string]any{"recovery_required": true}, "database_recovery_required"},
		{"checksums_disabled", map[string]any{"checksums": "off"}, "database_settings_unsafe"},
		{"checksums_missing", map[string]any{"checksums": nil}, "database_settings_unsafe"},
		{"backup_fresh", map[string]any{"backup": map[string]any{"ok": true, "checked_at": now.Add(-20 * time.Hour).Unix()}}, ""},
		{"backup_failed", map[string]any{"backup": map[string]any{"ok": false, "checked_at": now.Unix()}}, "database_backup_unavailable"},
		{"backup_expired", map[string]any{"backup": map[string]any{"ok": true, "checked_at": now.Add(-27 * time.Hour).Unix()}}, "database_backup_stale"},
		{"backup_unknown_age", map[string]any{"backup": map[string]any{"ok": true}}, "database_backup_stale"},
		{"backup_future", map[string]any{"backup": map[string]any{"ok": true, "checked_at": now.Add(time.Hour).Unix()}}, "database_backup_stale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := make(map[string]any)
			for k, v := range base {
				report[k] = v
			}
			for k, v := range tc.fields {
				report[k] = v
			}
			raw, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if got := validateIntegrityReport(raw, current, now); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
