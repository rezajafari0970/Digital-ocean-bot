package adminapi

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
)

func stabilityAuthorizationFixture(t *testing.T) (*tuningFixture, residentialperf.AdmissionEvidence, residentialperf.Request) {
	t.Helper()
	f, second, q := admissionFixture(t)
	first := earlierStability(second, 2*time.Minute)
	q.StabilityEvidenceID = first.ID
	for _, e := range []residentialperf.AdmissionEvidence{first, second} {
		if err := f.s.RecordAdmission(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	return f, second, q
}
func waitForAdmissionBlockedBy(t *testing.T, db *sql.DB, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("admission did not reach the controlled assignment wait")
}
func requireAdmissionRollback(t *testing.T, f *tuningFixture) {
	t.Helper()
	f.config(f.panels[0], f.before, 1)
	var clean bool
	err := f.db.QueryRow("SELECT tuning IS NULL AND NOT EXISTS(SELECT 1 FROM residential_performance_operations WHERE admission_evidence_id IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM residential_performance_targets WHERE experiment_id=$1 AND (tuning_id IS NOT NULL OR state<>'APPLIED' OR generation<>1)) FROM residential_performance_experiments WHERE id=$1", f.id).Scan(&clean)
	if err != nil || !clean {
		t.Fatal("rejected authorization left partial state", clean, err)
	}
}
func TestStabilityAuthorizationFreshnessAfterAssignmentWait(t *testing.T) {
	for _, kind := range []string{"native", "performance", "control", "fresh"} {
		t.Run(kind, func(t *testing.T) {
			f, e, q := stabilityAuthorizationFixture(t)
			switch kind {
			case "native":
				sqlMust(t, f.db, "UPDATE panel_routing_state SET verified_at=clock_timestamp()-interval '58.5 seconds' WHERE panel_id=$1", f.panels[0])
			case "performance":
				sqlMust(t, f.db, "UPDATE residential_performance_panels SET verified_at=clock_timestamp()-interval '58.5 seconds' WHERE panel_id=$1", f.panels[0])
			case "control":
				sqlMust(t, f.db, "UPDATE residential_proxies SET last_success_at=clock_timestamp()-interval '178.5 seconds' WHERE proxy_id=$1", e.Controls[0])
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			held, err := f.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Rollback()
			var pid int
			if err = held.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if _, err = held.ExecContext(ctx, "SELECT 1 FROM residential_performance_panels WHERE panel_id=$1 FOR UPDATE", f.panels[0]); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := f.s.Do(ctx, q); done <- err }()
			waitForAdmissionBlockedBy(t, f.db, pid)
			if kind != "fresh" {
				time.Sleep(1700 * time.Millisecond)
			}
			if err = held.Commit(); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if kind == "fresh" {
				if err != nil {
					t.Fatal("fresh authorization rejected", err)
				}
				f.phase("TESTING")
				return
			}
			if err == nil {
				t.Fatal("aged authorization committed", kind)
			}
			requireAdmissionRollback(t, f)
		})
	}
}
func TestStabilityLifecycleWritersBeforeAdmission(t *testing.T) {
	for _, kind := range []string{"expiry", "account-delete", "account-disable"} {
		t.Run(kind, func(t *testing.T) {
			f, _, q := stabilityAuthorizationFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			writer, err := f.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback()
			var query string
			switch kind {
			case "expiry":
				query = "UPDATE droplets SET expires_at=clock_timestamp()+interval '1 minute' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)"
			case "account-delete":
				query = "UPDATE accounts SET deletion_requested_at=clock_timestamp() WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1)"
			default:
				query = "UPDATE accounts SET enabled=false WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1)"
			}
			if _, err = writer.ExecContext(ctx, query, f.panels[0]); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, e := f.s.Do(ctx, q); done <- e }()
			select {
			case err = <-done:
				t.Fatal("admission ignored pending lifecycle writer", err)
			case <-time.After(100 * time.Millisecond):
			}
			if err = writer.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("invalidated lifecycle admitted")
			}
			requireAdmissionRollback(t, f)
		})
	}
}
func TestStabilityLifecycleWritersAfterEligibilityLocks(t *testing.T) {
	for _, kind := range []string{"expiry", "account-delete", "account-disable"} {
		t.Run(kind, func(t *testing.T) {
			f, _, q := stabilityAuthorizationFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			held, err := f.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Rollback()
			var pid int
			if err = held.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if _, err = held.ExecContext(ctx, "SELECT 1 FROM residential_performance_panels WHERE panel_id=$1 FOR UPDATE", f.panels[0]); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, e := f.s.Do(ctx, q); done <- e }()
			waitForAdmissionBlockedBy(t, f.db, pid)
			var query string
			switch kind {
			case "expiry":
				query = "UPDATE droplets SET expires_at=clock_timestamp()+interval '1 minute' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)"
			case "account-delete":
				query = "UPDATE accounts SET deletion_requested_at=clock_timestamp() WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1)"
			default:
				query = "UPDATE accounts SET enabled=false WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1)"
			}
			writerCtx, stop := context.WithTimeout(ctx, 200*time.Millisecond)
			_, err = f.db.ExecContext(writerCtx, query, f.panels[0])
			stop()
			if err == nil {
				t.Fatal("lifecycle writer bypassed admission eligibility lock")
			}
			if err = held.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal("serialized fresh admission failed", err)
			}
			f.phase("TESTING")
		})
	}
}
