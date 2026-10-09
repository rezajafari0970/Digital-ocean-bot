package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"sync"
	"testing"
	"time"
)

func earlierStability(e residentialperf.AdmissionEvidence, delta time.Duration) residentialperf.AdmissionEvidence {
	b, _ := json.Marshal(e)
	var x residentialperf.AdmissionEvidence
	_ = json.Unmarshal(b, &x)
	x.ID = perfUUID()
	x.Started = x.Started.Add(-delta)
	x.Finished = x.Finished.Add(-delta)
	for i := range x.Observations {
		x.Observations[i].Started = x.Observations[i].Started.Add(-delta)
		x.Observations[i].Finished = x.Observations[i].Finished.Add(-delta)
	}
	return x
}
func TestStabilityConsecutiveWindowsRejectUnsafe(t *testing.T) {
	for _, which := range []string{"first-missing", "first-healthy", "first-control-failed", "first-stale", "gap-short", "same-receipt", "role-change", "context-change", "destination-change", "intervening", "second-superseded", "wrong-action", "no-admission"} {
		t.Run(which, func(t *testing.T) {
			f, second, q := admissionFixture(t)
			ctx := context.Background()
			first := earlierStability(second, 2*time.Minute)
			switch which {
			case "first-healthy":
				for i := range first.Observations {
					o := &first.Observations[i]
					if o.ProxyID == first.Suspects[0] && o.Target == "gpt" {
						o.Outcome = "ok"
						o.CurlCode = 0
						o.HTTPStatus = 200
					}
				}
			case "first-control-failed":
				first.Observations[12].Outcome = "timeout"
				first.Observations[12].HTTPStatus = 0
			case "first-stale":
				first = earlierStability(second, 11*time.Minute)
			case "gap-short":
				first = earlierStability(second, 30*time.Second)
			case "role-change":
				first.Controls[0], first.Controls[1] = first.Controls[1], first.Controls[0]
				first.Suspects[0], first.Controls[0] = first.Controls[0], first.Suspects[0]
			case "context-change":
				first.Context.Generation++
			case "destination-change":
				for i := range second.Observations {
					o := &second.Observations[i]
					if o.ProxyID == second.Suspects[0] {
						if o.Target == "gpt" {
							o.Outcome = "ok"
							o.CurlCode = 0
							o.HTTPStatus = 200
						}
						if o.Target == "ads" {
							o.Outcome = "timeout"
							o.CurlCode = 28
							o.HTTPStatus = 0
						}
					}
				}
			}
			q.StabilityEvidenceID = first.ID
			if which != "first-missing" {
				if err := f.s.RecordAdmission(ctx, first); err != nil {
					t.Fatal(err)
				}
			}
			if which == "intervening" {
				x := earlierStability(second, time.Minute)
				if err := f.s.RecordAdmission(ctx, x); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.s.RecordAdmission(ctx, second); err != nil {
				t.Fatal(err)
			}
			switch which {
			case "same-receipt":
				q.StabilityEvidenceID = second.ID
			case "second-superseded":
				x := earlierStability(second, time.Second)
				if err := f.s.RecordAdmission(ctx, x); err != nil {
					t.Fatal(err)
				}
			case "wrong-action":
				q.Action = "tune_cancel"
			case "no-admission":
				q.AdmissionEvidenceID = ""
			}
			before, _ := f.s.Load(ctx, f.panels[0])
			if _, err := f.s.Do(ctx, q); err == nil {
				t.Fatal("unsafe paired evidence authorized")
			}
			after, _ := f.s.Load(ctx, f.panels[0])
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(after)
			if string(a) != string(b) {
				t.Fatal("rejected evidence changed assignment")
			}
		})
	}
}
func TestStabilityWindowsOneOperationReplayAndIsolation(t *testing.T) {
	f, second, q := admissionFixture(t)
	ctx := context.Background()
	first := earlierStability(second, 2*time.Minute)
	// Role ordering has no semantic significance; validator must not mutate input.
	first.Controls[0], first.Controls[1] = first.Controls[1], first.Controls[0]
	if err := f.s.RecordAdmission(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordAdmission(ctx, second); err != nil {
		t.Fatal(err)
	}
	q.StabilityEvidenceID = first.ID
	other, _ := f.s.Load(ctx, f.panels[1])
	receipt, err := f.s.Do(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	f.phase("TESTING")
	after, _ := f.s.Load(ctx, f.panels[1])
	a, _ := json.Marshal(other)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("unselected assignment changed")
	}
	newer := earlierStability(second, time.Second)
	if err = f.s.RecordAdmission(ctx, newer); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.Do(ctx, q)
	if err != nil || replay != receipt {
		t.Fatal("exact committed stability replay failed", err)
	}
	changed := q
	changed.StabilityEvidenceID = perfUUID()
	if _, err = f.s.Do(ctx, changed); err == nil {
		t.Fatal("different first evidence reused committed request")
	}
	var count int
	if err = f.db.QueryRow("SELECT count(*) FROM residential_performance_operations WHERE admission_evidence_id IS NOT NULL").Scan(&count); err != nil || count != 1 {
		t.Fatal("wrong admission operation count", count, err)
	}
	admissionVerify(t, f)
	f.tick()
	sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(now()-interval '1 minute')) WHERE id=$1", f.id)
	f.tick()
	f.phase("RESTORING")
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
}
func TestStabilityConcurrentStartsOnlyOneAdmission(t *testing.T) {
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
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		v := q
		v.RequestID = perfUUID()
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.Do(ctx, v); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("expected exactly one admission", success)
	}
}
