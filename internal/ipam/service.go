package ipam

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

type ObserveFunc func(context.Context, string) (providers.IPObservation, error)
type ReadResult struct {
	State       string                   `json:"state"`
	ErrorClass  string                   `json:"error_class,omitempty"`
	HTTPStatus  int                      `json:"http_status,omitempty"`
	Observation *providers.IPObservation `json:"observation,omitempty"`
}

// Refresh is the sole admission path used by the panel and inventory CLI.
// No retry or background schedule is implied by the next allowed attempt time.
func (s InventoryStore) Refresh(parent context.Context, account string, observe ObserveFunc) (ReadResult, error) {
	result := ReadResult{State: "FAILED"}
	ticket, e := s.Begin(parent, account)
	if e != nil {
		return result, e
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 40*time.Second)
	defer cancel()
	o, e := observe(ctx, account)
	retryAfter := time.Duration(0)
	if e == nil {
		e = providers.ValidateIPObservation(o)
	}
	if e == nil {
		result.State = "OBSERVED"
		result.Observation = &o
	} else {
		result.ErrorClass = "unavailable"
		var pe *providers.Error
		if errors.As(e, &pe) {
			result.ErrorClass = string(pe.Class)
			result.HTTPStatus = pe.StatusCode
			retryAfter = pe.RetryAfter
		}
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			result.ErrorClass = "transport"
		}
	}
	save, stop := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer stop()
	if e = s.Finish(save, ticket, result.Observation, result.ErrorClass, result.HTTPStatus, retryAfter); e != nil {
		return ReadResult{State: "UNCOMMITTED"}, e
	}
	return result, nil
}
