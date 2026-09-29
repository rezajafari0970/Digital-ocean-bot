package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

var ErrResourceNotReady = errors.New("provider resource not ready")

type ServerWaiter struct {
	Provider providers.ComputeDriver
	Timeout  time.Duration
}

func (w ServerWaiter) Wait(ctx context.Context, providerID string) (ResourceInfo, error) {
	if w.Provider == nil || providerID == "" {
		return ResourceInfo{}, ErrRuntimeConfig
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	attempt := 0
	for {
		attempt++
		x, err := w.Provider.GetServer(ctx, providerID)
		if err == nil && x.Ready && x.PrimaryIPv4 != "" {
			return ResourceInfo{ProviderID: x.ID, Host: x.PrimaryIPv4}, nil
		}
		if err != nil && !providers.IsClass(err, providers.ErrorNotFound) && !providers.IsRetryable(err) {
			return ResourceInfo{}, err
		}
		select {
		case <-ctx.Done():
			return ResourceInfo{}, ctx.Err()
		case <-deadline.C:
			return ResourceInfo{}, ErrResourceNotReady
		case <-time.After(waitDelay(attempt)):
		}
	}
}
