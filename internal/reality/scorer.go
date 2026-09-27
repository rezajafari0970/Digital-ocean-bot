package reality

import "sort"

func Evaluate(o Observation) Score {
	s := Score{Candidate: o.Candidate, Reason: "ineligible"}
	if !o.Reachable || !o.CertValid || o.TLSVersion != "TLSv1.3" || o.Samples <= 0 || o.Successes <= 0 {
		return s
	}
	x := append([]int64(nil), o.LatencyMS...)
	if len(x) == 0 {
		return s
	}
	sort.Slice(x, func(i, j int) bool { return x[i] < x[j] })
	med := x[len(x)/2]
	ratio := float64(o.Successes) / float64(o.Samples)
	s.Eligible = true
	s.MedianLatencyMS = med
	s.SuccessRatio = ratio
	s.Value = ratio*1000 - float64(med)
	s.Reason = "eligible"
	return s
}
func Rank(obs []Observation) []Score {
	out := make([]Score, 0, len(obs))
	for _, o := range obs {
		out = append(out, Evaluate(o))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Eligible != out[j].Eligible {
			return out[i].Eligible
		}
		if out[i].Value == out[j].Value {
			return out[i].Candidate.Target < out[j].Candidate.Target
		}
		return out[i].Value > out[j].Value
	})
	return out
}
