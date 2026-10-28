package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

func (s *Server) validateReplacementToken(ctx context.Context, accountID, providerName, token, mode, proxyID string) (providers.Account, error) {
	if mode != "direct" && mode != "proxy_required" {
		return providers.Account{}, errors.New("invalid_network_mode")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if providerName == "" {
		providerName = "digitalocean"
	}
	c := s.Container
	c.DB = s.DB
	client, closeFn, err := c.CredentialValidationClient(ctx, accountID)
	if err != nil {
		return providers.Account{}, err
	}
	defer closeFn()
	if s.Container.Providers == nil {
		return providers.Account{}, errors.New("provider_registry_unavailable")
	}
	d, err := s.Container.Providers.Open(ctx, providerName, providers.OpenRequest{AccountID: accountID, HTTPClient: client, Credentials: previewCredential{[]byte(token)}})
	if err != nil {
		return providers.Account{}, err
	}
	ar, ok := d.(providers.AccountReader)
	if !ok {
		return providers.Account{}, errors.New("provider_account_capability_missing")
	}
	return ar.Account(ctx)
}
