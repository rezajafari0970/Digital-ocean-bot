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
		password, err = s.Container.Secrets.GetProxy(r.Context(), cfg.Proxy.ID, cfg.ProxySecretRef)
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
	// Account-level proxy tests also refresh the persisted network identity.
	x := proxyWrite{Name: cfg.Proxy.Name, Host: cfg.Proxy.Host, Port: cfg.Proxy.Port, Username: cfg.ProxyUsername, Password: string(password)}
	if obs, obsErr := observeProxy(ctx, x, cfg.Proxy.Type); obsErr == nil {
		locale := localeForCountry(obs.CountryCode)
		_, _ = s.DB.ExecContext(ctx, `UPDATE proxies SET status='healthy',exit_ip=$2,country=$3,asn=$4,latency_ms=$5,last_checked_at=now(),last_success_at=now(),failure_count=0 WHERE id=$1`, cfg.Proxy.ID, obs.IP, obs.Country, obs.ASN, obs.LatencyMS)
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,timezone,locale,exit_ip,subnet_key,asn,country) VALUES($1,COALESCE(NULLIF($2,''),'UTC'),$3,$4::inet,CASE WHEN family($4::inet)=4 THEN host(network(set_masklen($4::inet,24)))||'/24' ELSE host(network(set_masklen($4::inet,48)))||'/48' END,$5,$6) ON CONFLICT(account_id) DO UPDATE SET timezone=EXCLUDED.timezone,locale=EXCLUDED.locale,exit_ip=EXCLUDED.exit_ip,subnet_key=EXCLUDED.subnet_key,asn=EXCLUDED.asn,country=EXCLUDED.country,updated_at=now()`, id, obs.Timezone, locale, obs.IP, obs.ASN, obs.Country)
	}
	writeJSON(w, 200, result)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
