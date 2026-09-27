package reality

import (
	"errors"
	"testing"
)

func obs(t string, ms int64) Observation {
	return Observation{Candidate: Candidate{Target: t, ServerName: t, Port: 443}, Reachable: true, TLSVersion: "TLSv1.3", CertValid: true, Samples: 3, Successes: 3, LatencyMS: []int64{ms, ms, ms}}
}
func TestStabilityRequiresHistory(t *testing.T) {
	s := Aggregate([]Observation{obs("a", 5)}, StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8})
	if _, _, e := ChooseStable(nil, s, StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8}); !errors.Is(e, ErrInsufficientHistory) {
		t.Fatalf("%v", e)
	}
}
func TestHysteresisKeepsCurrentForSmallGain(t *testing.T) {
	p := StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8, SwitchMargin: 5}
	all := []Observation{obs("a", 10), obs("a", 10), obs("a", 10), obs("b", 8), obs("b", 8), obs("b", 8)}
	s := Aggregate(all, p)
	cur := Evaluate(obs("a", 10))
	x, sw, e := ChooseStable(&cur, s, p)
	if e != nil || sw || x.Candidate.Target != "a" {
		t.Fatalf("x=%+v sw=%t e=%v", x, sw, e)
	}
}
func TestHysteresisSwitchesForMeaningfulGain(t *testing.T) {
	p := StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8, SwitchMargin: 5}
	all := []Observation{obs("a", 20), obs("a", 20), obs("a", 20), obs("b", 5), obs("b", 5), obs("b", 5)}
	s := Aggregate(all, p)
	cur := Evaluate(obs("a", 20))
	x, sw, e := ChooseStable(&cur, s, p)
	if e != nil || !sw || x.Candidate.Target != "b" {
		t.Fatalf("x=%+v sw=%t e=%v", x, sw, e)
	}
}
