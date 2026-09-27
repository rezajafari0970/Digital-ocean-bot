package listeners

import (
	"context"
	"errors"
)

var ErrCollectorUnavailable = errors.New("listener collector unavailable")

type Collector interface {
	Ports(context.Context) ([]int, error)
}

type RunFunc func(context.Context, string) (string, error)

type SSHCollector struct{ Run RunFunc }

func (c SSHCollector) Ports(ctx context.Context) ([]int, error) {
	if c.Run == nil {
		return nil, context.Canceled
	}
	out, err := c.Run(ctx, "ss -H -ltn 2>/dev/null")
	if err != nil {
		return nil, err
	}
	return ParseSS(out), nil
}
