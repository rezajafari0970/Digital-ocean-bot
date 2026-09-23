package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net/http"
	"time"
)

func (s *Server) testAccountProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg, err := s.Container.Accounts.Account(r.Context(), id)
	if err != nil || cfg.Proxy == nil {
		writeJSON(w, 409, map[string]string{"error": "proxy_not_assigned"})
		return
	}
	password := []byte(nil)
	if cfg.ProxySecretRef != "" {
		password, err = s.Container.Secrets.Get(r.Context(), id, cfg.ProxySecretRef)
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "proxy_secret_unavailable"})
			return
		}
		defer zeroBytes(password)
	}
	gateway, err := network.NewProxyGateway(id, *cfg.Proxy, network.ProxyCredentials{Username: cfg.ProxyUsername, Password: string(password)})
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "proxy_invalid"})
		return
	}
	defer gateway.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result := network.CheckProxy(ctx, gateway, "https://api.ipify.org?format=json", cfg.Proxy.ExitIP, "")
	writeJSON(w, 200, result)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
