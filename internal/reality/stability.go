package reality

import (
	"errors"
	"sort"
)

var ErrInsufficientHistory = errors.New("insufficient reality target history")

type StabilityPolicy struct {
	MinObservations  int
	MinEligibleRatio float64
	SwitchMargin     float64
}
type StableCandidate struct {
	Candidate       Candidate
	Observations    int
	EligibleRatio   float64
	MedianLatencyMS int64
	SuccessRatio    float64
	Score           float64
	Eligible        bool
}

func Aggregate(obs []Observation, p StabilityPolicy) []StableCandidate {
	type bucket struct {
		c Candidate
		o []Observation
	}
	m := map[string]*bucket{}
	for _, x := range obs {
		k := x.Candidate.Target + "\x00" + x.Candidate.ServerName + "\x00" + string(rune(x.Candidate.Port))
		if m[k] == nil {
			m[k] = &bucket{c: x.Candidate}
		}
		m[k].o = append(m[k].o, x)
	}
	out := make([]StableCandidate, 0, len(m))
	for _, b := range m {
		a := StableCandidate{Candidate: b.c, Observations: len(b.o)}
		lat := []int64{}
		eligible := 0
		ratio := 0.0
		for _, o := range b.o {
			s := Evaluate(o)
			if s.Eligible {
				eligible++
				lat = append(lat, s.MedianLatencyMS)
			}
			ratio += s.SuccessRatio
		}
		if len(b.o) > 0 {
			a.EligibleRatio = float64(eligible) / float64(len(b.o))
			a.SuccessRatio = ratio / float64(len(b.o))
		}
		if len(lat) > 0 {
			sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
			a.MedianLatencyMS = lat[len(lat)/2]
			if len(lat)%2 == 0 {
				a.MedianLatencyMS = (lat[len(lat)/2-1] + lat[len(lat)/2]) / 2
			}
		}
		a.Eligible = len(b.o) >= p.MinObservations && a.EligibleRatio >= p.MinEligibleRatio
		if a.Eligible {
			a.Score = a.SuccessRatio*1000 - float64(a.MedianLatencyMS)
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Eligible != out[j].Eligible {
			return out[i].Eligible
		}
		if out[i].Score == out[j].Score {
			return out[i].Candidate.Target < out[j].Candidate.Target
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func ChooseStable(current *Score, stable []StableCandidate, p StabilityPolicy) (StableCandidate, bool, error) {
	if len(stable) == 0 || !stable[0].Eligible {
		return StableCandidate{}, false, ErrInsufficientHistory
	}
	best := stable[0]
	if current == nil || !current.Eligible {
		return best, true, nil
	}
	for _, x := range stable {
		if x.Candidate == current.Candidate && x.Eligible {
			if best.Candidate == x.Candidate {
				return x, false, nil
			}
			if best.Score < x.Score+p.SwitchMargin {
				return x, false, nil
			}
			return best, true, nil
		}
	}
	return best, true, nil
}
