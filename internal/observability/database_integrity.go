package observability

import (
	"context"
	"encoding/json"
	"os"
	"syscall"
	"time"
)

type integrityWatermark struct {
	Database    string  `json:"database"`
	OID         string  `json:"oid"`
	Failures    int64   `json:"failures"`
	LastFailure float64 `json:"last_failure"`
	StatsReset  float64 `json:"stats_reset"`
}
type integrityReport struct {
	Backup *struct {
		OK bool `json:"ok"`
	} `json:"backup"`
	Version   int                `json:"version"`
	Status    string             `json:"status"`
	CheckedAt float64            `json:"checked_at"`
	Watermark integrityWatermark `json:"watermark"`
}

func validateIntegrityReport(raw []byte, current integrityWatermark, now time.Time) string {
	var report integrityReport
	if json.Unmarshal(raw, &report) != nil || report.Version != 1 {
		return "database_integrity_unverified"
	}
	if report.Status != "ok" {
		return "database_recovery_required"
	}
	if report.CheckedAt <= 0 || report.CheckedAt > float64(now.Add(time.Minute).Unix()) ||
		report.CheckedAt < float64(now.Add(-8*time.Hour).Unix()) {
		return "database_integrity_check_stale"
	}
	if report.Backup != nil && !report.Backup.OK {
		return "database_backup_unavailable"
	}
	if report.Watermark != current {
		return "database_recovery_required"
	}
	return ""
}
func (h Health) databaseIntegrity(ctx context.Context) string {
	if h.IntegrityReportPath == "" {
		return ""
	}
	var current integrityWatermark
	var safe bool
	err := h.DB.QueryRowContext(ctx, `SELECT current_database(),datid::text,
 COALESCE(checksum_failures,0),
 COALESCE(extract(epoch FROM checksum_last_failure),0)::float8,
 COALESCE(extract(epoch FROM stats_reset),0)::float8,
 current_setting('fsync')='on' AND current_setting('full_page_writes')='on'
 AND current_setting('synchronous_commit')<>'off'
 AND current_setting('zero_damaged_pages')='off'
 AND current_setting('ignore_checksum_failure')='off'
 FROM pg_stat_database WHERE datname=current_database()`).Scan(
		&current.Database, &current.OID, &current.Failures, &current.LastFailure, &current.StatsReset, &safe)
	if err != nil {
		return "database_integrity_unavailable"
	}
	if !safe {
		return "database_settings_unsafe"
	}
	raw, err := os.ReadFile(h.IntegrityReportPath)
	if err != nil {
		return "database_integrity_unverified"
	}
	if reason := validateIntegrityReport(raw, current, time.Now()); reason != "" {
		return reason
	}
	// Installation owns this directory on the same filesystem as the local DB.
	var fs syscall.Statfs_t
	if err := syscall.Statfs("/var/lib/postgresql", &fs); err != nil {
		return "database_disk_unavailable"
	}
	if fs.Blocks == 0 || uint64(fs.Bavail)*uint64(fs.Bsize) < 2<<30 || float64(fs.Bavail)/float64(fs.Blocks) < 0.05 {
		return "database_disk_space_low"
	}
	return ""
}
