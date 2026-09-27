package reality

import (
	"context"
	"errors"
	"time"
)

var ErrProbeCycle = errors.New("reality probe cycle failed")

type ObservationStore interface {
	Save(context.Context, string, Observation) error
	Recent(context.Context, string, int) ([]Observation, error)
	Select(context.Context, string, Score, time.Time) error
}

type CycleResult struct {
	Observed int
	Failed   int
	Stable   bool
	Switched bool
	Selected string
}

func RunPanel(ctx context.Context, panelID string, p Prober, candidates []Candidate, samples, history int, policy StabilityPolicy, store ObservationStore, current *Score) (CycleResult, error) {
	if panelID == "" || p == nil || store == nil || samples < 1 || history < 1 {
		return CycleResult{}, ErrProbeCycle
	}
	r := CycleResult{}
	for _, c := range candidates {
		o, err := p.Probe(ctx, c, samples)
		if err != nil {
			r.Failed++
			continue
		}
		if err = store.Save(ctx, panelID, o); err != nil {
			return r, err
		}
		r.Observed++
	}
	if r.Observed == 0 {
		return r, ErrProbeCycle
	}
	hist, err := store.Recent(ctx, panelID, history)
	if err != nil {
		return r, err
	}
	stable := Aggregate(hist, policy)
	choice, sw, err := ChooseStable(current, stable, policy)
	if errors.Is(err, ErrInsufficientHistory) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	r.Stable = true
	r.Switched = sw
	r.Selected = choice.Candidate.Target
	if current == nil || sw {
		score := Score{Candidate: choice.Candidate, Eligible: true, MedianLatencyMS: choice.MedianLatencyMS, SuccessRatio: choice.SuccessRatio, Value: choice.Score, Reason: "stable"}
		if err = store.Select(ctx, panelID, score, time.Now().UTC()); err != nil {
			return r, err
		}
	}
	return r, nil
}
