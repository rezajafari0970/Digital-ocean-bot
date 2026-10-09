package residentialperf

import (
	"encoding/json"
	"testing"
	"time"
)

func healthFixture(now time.Time) AdmissionEvidence {
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	c := AdmissionContext{PanelID: "44444444-4444-4444-8444-444444444444", Plan: "plan", Generation: 1, Proxies: map[string]int64{}}
	for _, id := range ids {
		c.Proxies[id] = 1
	}
	e := AdmissionEvidence{ID: "55555555-5555-4555-8555-555555555555", Manifest: AdmissionManifest, Context: c, Suspects: ids[:1], Controls: ids[1:], Started: now.Add(-time.Minute), Finished: now.Add(-time.Second), ContextStable: true, ChainVerified: true, CollectionHealthy: true, CollectionOutcome: "completed", Guard: AdmissionGuard{Startup: "ready", NegativeCoreAlive: true, Positive: &AdmissionObservation{Outcome: "ok", HTTPStatus: 204}, Denied: &AdmissionObservation{Outcome: "transport", CurlCode: 56}}}
	for _, id := range ids {
		for _, target := range AdmissionTargets {
			for round := 0; round < 3; round++ {
				e.Observations = append(e.Observations, AdmissionObservation{ProxyID: id, Target: target.Name, Round: round, Started: e.Started, Finished: e.Finished, Milliseconds: 59000, Outcome: "ok", HTTPStatus: target.Status})
			}
		}
	}
	return e
}
func TestObservedHealthPreservesFailuresWithoutAuthorizingMutation(t *testing.T) {
	now := time.Now().UTC()
	e := healthFixture(now)
	e.Observations[0].Outcome = "timeout"
	e.Observations[0].CurlCode = 28
	e.Observations[0].HTTPStatus = 0
	e.Observations[12].Outcome = "timeout"
	e.Observations[12].CurlCode = 28
	e.Observations[12].HTTPStatus = 0 // A failed control must remain visible.
	before, _ := json.Marshal(e)
	v := ObserveHealth(e.Context, &e, now)
	after, _ := json.Marshal(e)
	if v.State != "CURRENT_DIAGNOSTIC" || v.MutationAllowed || v.NewProbeRequests != 0 || v.ProviderIndependenceVerified || len(v.Rows) != 12 {
		t.Fatalf("bad view: %+v", v)
	}
	failed := 0
	for _, row := range v.Rows {
		failed += row.Failed
		if row.Failed > 0 && row.State != "OBSERVED_FAILURE" {
			t.Fatal("failure hidden")
		}
	}
	if failed != 2 || string(before) != string(after) {
		t.Fatal("evidence lost or changed")
	}
}
func TestObservedHealthRejectsStaleChangedIncompleteOrInconsistentEvidence(t *testing.T) {
	now := time.Now().UTC()
	for _, kind := range []string{"stale", "context", "duplicate", "incomplete", "inconsistent", "future", "collector", "guard"} {
		t.Run(kind, func(t *testing.T) {
			e := healthFixture(now)
			current := e.Context
			want := "INVALID_EVIDENCE"
			at := now
			switch kind {
			case "stale":
				at = now.Add(6 * time.Minute)
				want = "STALE"
			case "context":
				current.Plan = "changed"
				want = "CONTEXT_CHANGED"
			case "duplicate":
				e.Observations[1] = e.Observations[0]
			case "incomplete":
				e.Observations = e.Observations[:35]
			case "inconsistent":
				e.Observations[0].CurlCode = 28
			case "future":
				e.Finished = now.Add(time.Minute)
			case "collector":
				e.CollectionHealthy = false
			case "guard":
				e.Guard.NegativeCoreAlive = false
			}
			v := ObserveHealth(current, &e, at)
			if v.State != want || v.MutationAllowed {
				t.Fatalf("%s %+v", want, v)
			}
			for _, row := range v.Rows {
				if row.State == "OBSERVED_PASS" {
					t.Fatal("old/invalid observation treated as current")
				}
			}
		})
	}
	e := healthFixture(now)
	v := ObserveHealth(e.Context, nil, now)
	if v.State != "NO_EVIDENCE" || len(v.Rows) != 0 || v.MutationAllowed {
		t.Fatal(v)
	}
}
