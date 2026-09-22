package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"strconv"
	"time"
)

var ErrResourceNotReady = errors.New("provider resource not ready")

type DropletLookup interface {
	ListDropletModels(context.Context) ([]digitalocean.Droplet, error)
}
type DigitalOceanWaiter struct {
	Provider DropletLookup
	Timeout  time.Duration
}

func (w DigitalOceanWaiter) Wait(ctx context.Context, providerID string) (ResourceInfo, error) {
	id, err := strconv.Atoi(providerID)
	if err != nil {
		return ResourceInfo{}, err
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
		items, err := w.Provider.ListDropletModels(ctx)
		if err == nil {
			for _, x := range items {
				if x.ID == id && x.Status == "active" {
					host := dropletIPv4(x)
					if host != "" {
						return ResourceInfo{ProviderID: providerID, Host: host}, nil
					}
				}
			}
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

func dropletIPv4(d digitalocean.Droplet) string { return d.PublicIPv4 }
