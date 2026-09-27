package reality

import "testing"

func TestRankPrefersReliableLowLatency(t *testing.T) {
	a := Observation{Candidate: Candidate{Target: "a", ServerName: "a", Port: 443}, Reachable: true, TLSVersion: "TLSv1.3", CertValid: true, Samples: 3, Successes: 3, LatencyMS: []int64{30, 20, 25}}
	b := a
	b.Candidate.Target = "b"
	b.Candidate.ServerName = "b"
	b.Successes = 2
	b.LatencyMS = []int64{10, 12}
	r := Rank([]Observation{b, a})
	if r[0].Candidate.Target != "a" || !r[0].Eligible {
		t.Fatalf("%+v", r)
	}
}
func TestRejectsInvalidCertificate(t *testing.T) {
	s := Evaluate(Observation{Candidate: Candidate{Target: "x"}, Reachable: true, TLSVersion: "TLSv1.3", CertValid: false, Samples: 1, Successes: 1, LatencyMS: []int64{1}})
	if s.Eligible {
		t.Fatal("expected ineligible")
	}
}
