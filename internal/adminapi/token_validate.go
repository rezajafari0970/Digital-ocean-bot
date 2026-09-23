package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"net/http"
)

func (s *Server) validateReplacementToken(ctx context.Context, accountID, token, mode, proxyID string) (digitalocean.DiscoveryResult, error) {
	cell := accounts.NewCellManager().Register("replace-" + accountID)
	var client *http.Client
	var closeFn func()
	if mode == "proxy_required" {
		var px network.Proxy
		var user, ref string
		if err := s.DB.QueryRowContext(ctx, `SELECT id::text,name,type,host,port,COALESCE(username,''),COALESCE(secret_ref,''),status FROM proxies WHERE id=$1`, proxyID).Scan(&px.ID, &px.Name, &px.Type, &px.Host, &px.Port, &user, &ref, &px.Status); err != nil {
			return digitalocean.DiscoveryResult{}, err
		}
		if px.Status != network.StatusHealthy && px.Status != network.StatusDegraded {
			return digitalocean.DiscoveryResult{}, errors.New("proxy_not_healthy")
		}
		var pass []byte
		if ref != "" {
			var err error
			pass, err = s.Container.Secrets.GetProxy(ctx, px.ID, ref)
			if err != nil {
				return digitalocean.DiscoveryResult{}, err
			}
			defer zeroBytes(pass)
		}
		g, err := network.NewProxyGateway("replace-"+accountID, px, network.ProxyCredentials{Username: user, Password: string(pass)})
		if err != nil {
			return digitalocean.DiscoveryResult{}, err
		}
		client = g.Client
		closeFn = g.CloseIdleConnections
	} else {
		b, err := network.NewIsolatedDirectClient("replace-" + accountID)
		if err != nil {
			return digitalocean.DiscoveryResult{}, err
		}
		client = b.Client
		closeFn = b.CloseIdleConnections
	}
	defer closeFn()
	provider, err := digitalocean.NewClient(cell.Context, "candidate-token", memorySecret{[]byte(token)}, client)
	if err != nil {
		return digitalocean.DiscoveryResult{}, err
	}
	return provider.Discover(ctx)
}
