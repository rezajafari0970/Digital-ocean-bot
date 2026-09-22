package resilience

import (
	"context"
	"time"
)

type Classifier func(error) ErrorClass
type Retrier struct {
	Policy   Policy
	Circuit  *Circuit
	Classify Classifier
}

func (r Retrier) Do(ctx context.Context, fn func() error) error {
	p := r.Policy
	if p.MaxAttempts < 1 {
		p = DefaultPolicy()
	}
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if r.Circuit != nil {
			if err := r.Circuit.Allow(time.Now()); err != nil {
				return err
			}
		}
		err := fn()
		if err == nil {
			if r.Circuit != nil {
				r.Circuit.Success()
			}
			return nil
		}
		class := Unknown
		if r.Classify != nil {
			class = r.Classify(err)
		}
		if class == Permanent {
			return err
		}
		if r.Circuit != nil {
			r.Circuit.Failure(time.Now())
		}
		if attempt == p.MaxAttempts {
			return err
		}
		delay := p.Delay(attempt)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil
}
