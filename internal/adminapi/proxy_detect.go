package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"io"
	"net/http"
	"time"
)

var errProxyUndetected = errors.New("proxy protocol could not be detected")

func detectProxy(ctx context.Context, x proxyWrite) (network.ProxyType, error) {
	return detectProxyTypes(ctx, x, []network.ProxyType{network.ProxyHTTP, network.ProxyHTTPS, network.ProxySOCKS5})
}
func detectProxyTypes(ctx context.Context, x proxyWrite, types []network.ProxyType) (network.ProxyType, error) {
	for _, typ := range types {
		p := network.Proxy{Name: x.Name, Type: typ, Host: x.Host, Port: x.Port, Status: network.StatusHealthy}
		g, err := network.NewProxyGateway("detect", p, network.ProxyCredentials{Username: x.Username, Password: x.Password})
		if err != nil {
			continue
		}
		testCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		req, _ := http.NewRequestWithContext(testCtx, http.MethodGet, "https://api.ipify.org?format=json", nil)
		resp, err := g.Client.Do(req)
		cancel()
		g.CloseIdleConnections()
		if err == nil && resp != nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return typ, nil
			}
		}
	}
	return "", errProxyUndetected
}
