package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"testing"
	"time"
)

func TestStabilityExactReadOnlyReconciliation(t *testing.T) {
	f, second, q := admissionFixture(t)
	ctx := context.Background()
	first := earlierStability(second, 2*time.Minute)
	if err := f.s.RecordAdmission(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordAdmission(ctx, second); err != nil {
		t.Fatal(err)
	}
	q.StabilityEvidenceID = first.ID
	absent, err := f.s.Reconcile(ctx, q)
	if err != nil || absent != nil {
		t.Fatal("unexpected uncommitted receipt", err)
	}
	committed, err := f.s.Do(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.s.Reconcile(ctx, q)
	if err != nil || got == nil || *got != committed {
		t.Fatal("exact reconciliation failed", err)
	}
	for _, kind := range []string{"first", "duration", "action"} {
		changed := q
		switch kind {
		case "first":
			changed.StabilityEvidenceID = perfUUID()
		case "duration":
			changed.Minutes = 6
		case "action":
			changed.Action = "tune_cancel"
			changed.StabilityEvidenceID = ""
			changed.AdmissionEvidenceID = ""
		}
		if _, err = f.s.Reconcile(ctx, changed); err == nil {
			t.Fatal("different request adopted", kind)
		}
	}
	var count int
	if err = f.db.QueryRow("SELECT count(*) FROM residential_performance_operations WHERE admission_evidence_id IS NOT NULL").Scan(&count); err != nil || count != 1 {
		t.Fatal("reconciliation mutated operations", count, err)
	}
	f.phase("TESTING")
}

func TestStabilityAmbiguousRecordingOrderRejected(t *testing.T) {
	f, second, q := admissionFixture(t)
	first := earlierStability(second, 2*time.Minute)
	q.StabilityEvidenceID = first.ID
	// Force both possible UUID tie orders. Neither may be accepted as causal order.
	for _, e := range []residentialperf.AdmissionEvidence{first, second} {
		raw, _ := json.Marshal(e)
		sqlMust(t, f.db, "INSERT INTO residential_admission_evidence(id,panel_id,evidence,recorded_at) VALUES($1,$2,$3,'2026-01-01T00:00:00Z')", e.ID, e.Context.PanelID, string(raw))
	}
	if _, err := f.s.Do(context.Background(), q); err == nil {
		t.Fatal("timestamp tie admitted")
	}
	f.config(f.panels[0], f.before, 1)
}

func TestStabilityFreshnessAfterPerformanceLockWait(t *testing.T) {
	f, second, q := admissionFixture(t)
	ctx := context.Background()
	first := earlierStability(second, 9*time.Minute+57500*time.Millisecond)
	q.StabilityEvidenceID = first.ID
	if err := f.s.RecordAdmission(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordAdmission(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := residentialperf.ValidateStabilityPair(first, second, time.Now()); err != nil {
		t.Fatal("fixture already stale", err)
	}
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = residentialperf.Lock(ctx, tx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := f.s.Do(ctx, q); done <- e }()
	time.Sleep(2200 * time.Millisecond)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("pre-wait timestamp authorized stale first window")
	}
}

func TestAdmissionConcurrentContextWritersAreSerialized(t *testing.T) {
	for _, kind := range []string{"insert-enabled-proxy", "native-plan"} {
		t.Run(kind, func(t *testing.T) {
			f, e, q := admissionFixture(t)
			ctx := context.Background()
			if err := f.s.RecordAdmission(ctx, e); err != nil {
				t.Fatal(err)
			}
			tx, err := f.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if kind == "insert-enabled-proxy" {
				id := perfUUID()
				_, err = tx.ExecContext(ctx, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1::uuid,$1::text,'socks5','localhost',1080,'healthy',now(),'residential-ads-'||$1)", id)
			} else {
				_, err = tx.ExecContext(ctx, "UPDATE panel_routing_state SET plan_hash='changed-at-boundary' WHERE panel_id=$1", f.panels[0])
			}
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, e := f.s.Do(ctx, q); done <- e }()
			select {
			case e := <-done:
				t.Fatal("admission ignored pending context writer", e)
			case <-time.After(100 * time.Millisecond):
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("changed context admitted")
			}
		})
	}
}

func TestAdmissionControlsRequireConsistentTransport(t *testing.T) {
	_, e, _ := admissionFixture(t)
	for index, o := range e.Observations {
		if o.ProxyID == e.Suspects[0] {
			continue
		}
		for _, code := range []int{7, 28, 35, 56, 67} {
			b, _ := json.Marshal(e)
			var changed residentialperf.AdmissionEvidence
			json.Unmarshal(b, &changed)
			changed.Observations[index].CurlCode = code
			if err := changed.Eligible(time.Now()); err == nil {
				t.Fatal("nonzero curl accepted as a successful control", index, code)
			}
		}
	}
}

func TestStabilityRecordingOrderUsesLockTimeNotTransactionBegin(t *testing.T) {
	f, second, q := admissionFixture(t)
	ctx := context.Background()
	first := earlierStability(second, 2*time.Minute)
	q.StabilityEvidenceID = first.ID
	early, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer early.Rollback()
	var began time.Time
	if err = early.QueryRowContext(ctx, "SELECT now()").Scan(&began); err != nil {
		t.Fatal(err)
	}
	if err = f.s.RecordAdmission(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err = residentialperf.Lock(ctx, early); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(second)
	if _, err = early.ExecContext(ctx, "INSERT INTO residential_admission_evidence(id,panel_id,evidence) VALUES($1,$2,$3)", second.ID, second.Context.PanelID, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err = early.Commit(); err != nil {
		t.Fatal(err)
	}
	var latest string
	if err = f.db.QueryRow("SELECT id::text FROM residential_admission_evidence ORDER BY recorded_at DESC,id DESC LIMIT 1").Scan(&latest); err != nil || latest != second.ID {
		t.Fatal("transaction start displaced recording order", err)
	}
	if _, err = f.s.Do(ctx, q); err != nil {
		t.Fatal("causally consecutive pair rejected", err)
	}
}

func TestStabilityFreshnessAfterAssignmentLockWait(t *testing.T) {
	f, second, q := admissionFixture(t)
	ctx := context.Background()
	first := earlierStability(second, 9*time.Minute+57500*time.Millisecond)
	q.StabilityEvidenceID = first.ID
	if err := f.s.RecordAdmission(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordAdmission(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := residentialperf.ValidateStabilityPair(first, second, time.Now()); err != nil {
		t.Fatal("fixture already stale", err)
	}
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT 1 FROM residential_performance_panels WHERE panel_id=$1 FOR UPDATE", f.panels[0]); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := f.s.Do(ctx, q); done <- e }()
	time.Sleep(2200 * time.Millisecond)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("pre-wait timestamp authorized stale first window")
	}
}
