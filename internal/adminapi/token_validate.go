package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"time"
)

func (s *Server) validateReplacementToken(ctx context.Context, accountID, providerName, token, mode, proxyID string) (providers.Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if providerName == "" {
		providerName = "digitalocean"
	}
	var client *http.Client
	var closeFn func()
	if mode == "proxy_required" {
		var px network.Proxy
		var user, ref string
		if err := s.DB.QueryRowContext(ctx, `SELECT id::text,name,type,host,port,COALESCE(username,''),COALESCE(secret_ref,''),status FROM proxies WHERE id=$1`, proxyID).Scan(&px.ID, &px.Name, &px.Type, &px.Host, &px.Port, &user, &ref, &px.Status); err != nil {
			return providers.Account{}, err
		}
		if px.Status != network.StatusHealthy {
			return providers.Account{}, errors.New("proxy_not_healthy")
		}
		var pass []byte
		if ref != "" {
			var err error
			pass, err = s.Container.Secrets.GetProxy(ctx, px.ID, ref)
			if err != nil {
				return providers.Account{}, err
			}
			defer zeroBytes(pass)
		}
		g, err := network.NewProxyGateway("replace-"+accountID, px, network.ProxyCredentials{Username: user, Password: string(pass)})
		if err != nil {
			return providers.Account{}, err
		}
		client = g.Client
		closeFn = g.CloseIdleConnections
	} else {
		b, err := network.NewIsolatedDirectClient("replace-" + accountID)
		if err != nil {
			return providers.Account{}, err
		}
		client = b.Client
		closeFn = b.CloseIdleConnections
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
