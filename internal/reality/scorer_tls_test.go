package reality

import "testing"

func TestEvaluateAcceptsTLS13Spellings(t *testing.T) {
	for _, v := range []string{"1.3", "TLS1.3", "TLSv1.3", " tlsv1.3 "} {
		s := Evaluate(Observation{Candidate: Candidate{Target: "x", ServerName: "x", Port: 443}, Reachable: true, CertValid: true, TLSVersion: v, Samples: 1, Successes: 1, LatencyMS: []int64{10}})
		if !s.Eligible {
			t.Fatalf("%q should be eligible", v)
		}
	}
}
func TestEvaluateRejectsTLS12(t *testing.T) {
	s := Evaluate(Observation{Reachable: true, CertValid: true, TLSVersion: "1.2", Samples: 1, Successes: 1, LatencyMS: []int64{10}})
	if s.Eligible {
		t.Fatal("TLS 1.2 must not be eligible")
	}
}
