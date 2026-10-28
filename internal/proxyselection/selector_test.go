package proxyselection

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCountryPriorityExhaustsPeersBeforeFallback(t *testing.T) {
	providers := []Provider{{"one", 2}, {"two", 2}}
	state := Checkpoint{}
	seen := []Request{}
	probe := func(_ context.Context, r Request) Evaluation {
		seen = append(seen, r)
		if r.Provider == "two" && r.Attempt == 1 {
			return Evaluation{Health: Usable, Candidate: "proof"}
		}
		return Evaluation{Health: Unusable}
	}
	d := (Selector{}).Select(context.Background(), "a", []string{"US", "TR", Auto}, providers, &state, probe)
	if d.Kind != "selected" || d.Country != "US" || d.Provider != "two" {
		t.Fatal(d)
	}
	want := []Request{{"US", "one", 0}, {"US", "two", 0}, {"US", "one", 1}, {"US", "two", 1}}
	if !reflect.DeepEqual(seen, want) {
		t.Fatal(seen)
	}
}
func TestCountryPriorityUnknownNeverAuthorizesLowerCountry(t *testing.T) {
	state := Checkpoint{}
	providers := []Provider{{"one", 1}, {"two", 1}}
	seen := []string{}
	d := (Selector{}).Select(context.Background(), "a", []string{"US", "TR", Auto}, providers, &state, func(_ context.Context, r Request) Evaluation {
		seen = append(seen, r.Country)
		return Evaluation{Health: Unknown}
	})
	if d.Kind != "pending" || !reflect.DeepEqual(seen, []string{"US", "US"}) {
		t.Fatal(d, seen)
	}
	// A healthy peer can still win when another provider's geo observer is down.
	d = (Selector{}).Select(context.Background(), "a", []string{"US", "TR"}, providers, &state, func(_ context.Context, r Request) Evaluation {
		if r.Provider == "two" {
			return Evaluation{Health: Usable}
		}
		return Evaluation{Health: Unknown}
	})
	if d.Kind != "selected" || d.Country != "US" {
		t.Fatal(d)
	}
}
func TestCountryPriorityResumeScopeAndFailback(t *testing.T) {
	now := time.Unix(1000, 0)
	sel := Selector{Now: func() time.Time { return now }}
	state := Checkpoint{}
	providers := []Provider{{"one", 2}, {"two", 1}}
	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	d := sel.Select(ctx, "a", []string{"US", "TR", Auto}, providers, &state, func(_ context.Context, r Request) Evaluation {
		count++
		if count == 2 {
			cancel()
		}
		return Evaluation{Health: Unusable}
	})
	if d.Kind != "pending" || state.Countries["US"].Attempts["one"] != 1 || state.Countries["US"].Attempts["two"] != 0 {
		t.Fatal(d, state)
	}
	raw, _ := json.Marshal(state)
	state = Checkpoint{}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	seen := []Request{}
	fail := func(_ context.Context, r Request) Evaluation {
		seen = append(seen, r)
		return Evaluation{Health: Unusable}
	}
	d = sel.Select(context.Background(), "a", []string{"US", "TR", Auto}, providers, &state, fail)
	if d.Kind != "legacy" || seen[0] != (Request{"US", "one", 1}) {
		t.Fatal(d, seen)
	}
	now = now.Add(31 * time.Second)
	d = sel.Select(context.Background(), "a", []string{"US", "TR", Auto}, providers, &state, func(_ context.Context, r Request) Evaluation { return Evaluation{Health: Usable} })
	if d.Kind != "selected" || d.Country != "US" {
		t.Fatal("no failback", d)
	}
	seen = nil
	sel.Select(context.Background(), "new-policy-or-pool", []string{"US"}, providers, &state, fail)
	if seen[0].Attempt != 0 {
		t.Fatal("stale scope reused", seen)
	}
}
func TestCountryPriorityValidation(t *testing.T) {
	for _, bad := range [][]string{{"US", "us"}, {"AUTO", "US"}, {"ZZ"}, {"UK", "GB"}} {
		if _, err := Normalize(bad); err == nil {
			t.Fatal(bad)
		}
	}
	got, err := Normalize([]string{" us ", "tr", Auto})
	if err != nil || !reflect.DeepEqual(got, []string{"US", "TR", Auto}) {
		t.Fatal(got, err)
	}
	if got, err = Normalize(nil); err != nil || !Legacy(got) {
		t.Fatal(got, err)
	}
}

func TestCountryPriorityLongSweepCannotStarveAuto(t *testing.T) {
	now := time.Unix(1000, 0)
	sel := Selector{Now: func() time.Time { return now }}
	state := Checkpoint{}
	calls := map[string]int{}
	var final Decision
	for tick := 0; tick < 10; tick++ {
		ctx, cancel := context.WithCancel(context.Background())
		batch := 0
		final = sel.Select(ctx, "stable", []string{"US", "TR", Auto}, []Provider{{"one", 6}}, &state, func(_ context.Context, r Request) Evaluation {
			batch++
			now = now.Add(7 * time.Second)
			if batch == 4 {
				cancel()
				return Evaluation{Health: Unknown}
			}
			calls[r.Country]++
			return Evaluation{Health: Unusable}
		})
		cancel()
		if final.Kind == "legacy" {
			break
		}
	}
	if final.Kind != "legacy" || calls["US"] != 6 || calls["TR"] != 6 {
		t.Fatal("negative evidence expired while lower sweep was progressing", final, calls)
	}
	now = now.Add(31 * time.Second)
	final = sel.Select(context.Background(), "stable", []string{"US", "TR", Auto}, []Provider{{"one", 6}}, &state, func(_ context.Context, r Request) Evaluation { return Evaluation{Health: Usable} })
	if final.Country != "US" {
		t.Fatal("completed sweep stopped failback", final)
	}
}

func TestCountryReviewTwoLargeFiniteSweepCompletes(t *testing.T) {
	now := time.Now()
	selector := Selector{Now: func() time.Time { return now }}
	state := Checkpoint{}
	providers := []Provider{}
	for i := 0; i < 8; i++ {
		providers = append(providers, Provider{ID: string(rune('a' + i)), Attempts: 6})
	}
	calls := 0
	decision := selector.Select(context.Background(), "large", []string{"US", "AUTO"}, providers, &state, func(context.Context, Request) Evaluation {
		calls++
		now = now.Add(6500 * time.Millisecond)
		return Evaluation{Health: Unusable}
	})
	if calls != 48 || decision.Kind != "legacy" {
		t.Fatal("finite sweep repeatedly expires", calls, decision)
	}
}
