package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

func (c Container) newIdentityGateway(ctx context.Context, account string, p network.Proxy, creds network.ProxyCredentials) (*network.Gateway, error) {
	enabled, err := c.Economy.Enabled(ctx, account)
	if err != nil {
		return nil, err
	}
	return network.NewIdentityProxyGateway(account, p, creds, enabled)
}
