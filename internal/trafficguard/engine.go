package trafficguard

import (
	"context"
	"errors"
	"time"
)

var ErrClientMismatch = errors.New("traffic client mismatch")

type TrafficSource interface {
	Get(context.Context, string) (int64, int64, error)
}
type Disabler interface {
	Disable(context.Context, string) error
}
type Engine struct {
	Store    Store
	Source   TrafficSource
	Disabler Disabler
	Policy   Policy
}

func (e Engine) Check(ctx context.Context, clientID, email string) (Decision, error) {
	up, down, err := e.Source.Get(ctx, email)
	if err != nil {
		return Decision{}, err
	}
	current := Sample{ClientID: clientID, UpBytes: up, DownBytes: down, At: time.Now().UTC()}
	previous, err := e.Store.LastSample(ctx, clientID)
	if errors.Is(err, ErrNoSample) {
		return Decision{}, e.Store.SaveSample(ctx, current)
	}
	if err != nil {
		return Decision{}, err
	}
	baseline, err := e.Store.LoadBaseline(ctx, clientID)
	if err != nil {
		return Decision{}, err
	}
	baseline.ClientID = clientID
	baseline, decision := Analyze(previous, current, baseline, e.Policy)
	if err := e.Store.SaveSample(ctx, current); err != nil {
		return decision, err
	}
	if err := e.Store.SaveBaseline(ctx, baseline); err != nil {
		return decision, err
	}
	if err := e.Store.SaveDecision(ctx, clientID, decision); err != nil {
		return decision, err
	}
	if decision.Confirmed && decision.Action == ActionDisable && e.Disabler != nil {
		if err := e.Disabler.Disable(ctx, clientID); err != nil {
			return decision, err
		}
	}
	return decision, nil
}
