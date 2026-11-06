// Package builtin is the composition root for the compiled provider plugins.
// Application orchestration depends on provider contracts, not concrete drivers.
package builtin

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/kamatera"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/linode"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
)

func NewRegistry() (*providers.Registry, error) {
	registry := providers.NewRegistry()
	for _, factory := range []providers.Factory{digitalocean.Factory{}, vultr.Factory{}, upcloud.Factory{}, aws.Factory{}, ovh.Factory{}, linode.Factory{}, kamatera.Factory{}} {
		if err := registry.Register(factory); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
