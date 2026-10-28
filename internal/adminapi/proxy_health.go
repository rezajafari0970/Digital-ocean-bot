package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
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
		password, err = s.Container.Secrets.GetProxyAtVersion(r.Context(), cfg.Proxy.ID, cfg.ProxySecretRef, cfg.ProxySecretVersion)
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "proxy_secret_unavailable"})
			return
		}
		defer zeroBytes(password)
	}
	gateway, err := network.NewProxyProbeGateway(id, *cfg.Proxy, network.ProxyCredentials{Username: cfg.ProxyUsername, Password: string(password)})
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "proxy_invalid"})
		return
	}
	defer gateway.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	// Account-level proxy tests are read-only for account identity. Sticky Keeper is the sole writer.
	x := proxyWrite{Name: cfg.Proxy.Name, Host: cfg.Proxy.Host, Port: cfg.Proxy.Port, Username: cfg.ProxyUsername, Password: string(password)}
	obs, obsErr := observeProxy(ctx, x, cfg.Proxy.Type)
	if err = s.saveAccountProxyObservation(ctx, cfg, obs, obsErr); err != nil {
		if errors.Is(err, errProxyObservationStale) {
			writeJSON(w, 409, map[string]string{"error": "proxy_changed_during_test"})
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}

	if obsErr != nil {
		writeJSON(w, 502, map[string]string{"status": "down", "error": "proxy_observation_failed"})
		return
	}
	locale := localeForCountry(obs.CountryCode)

	providerUsable := false
	providerError := ""
	if rt, rtErr := s.Container.Runtime(ctx, id); rtErr == nil {
		providerErr := rt.Driver.Health(ctx)
		if rt.Gateway != nil {
			rt.Gateway.CloseIdleConnections()
		}
		providerUsable = providerErr == nil
		if providerErr != nil {
			providerError = providerErr.Error()
		}
	} else {
		providerError = rtErr.Error()
	}
	writeJSON(w, 200, map[string]any{"status": "healthy", "provider_usable": providerUsable, "provider_error": providerError, "exit_ip": obs.IP, "country": obs.Country, "country_code": obs.CountryCode, "timezone": obs.Timezone, "locale": locale, "asn": obs.ASN, "latency_ms": obs.LatencyMS})
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

var errProxyObservationStale = errors.New("proxy changed during observation")

func (s *Server) saveAccountProxyObservation(ctx context.Context, cfg app.AccountConfig, obs ProxyObservation, obsErr error) error {
	var err error
	const versionPredicate = ` AND settings_revision=$7 AND COALESCE((SELECT sec.xmin::text||':'||sec.updated_at::text FROM secrets sec WHERE sec.proxy_id=proxies.id AND sec.id=proxies.secret_ref),'')=$8`
	var result sql.Result
	if obsErr != nil {
		result, err = s.DB.ExecContext(ctx, `UPDATE proxies SET last_checked_at=now(),failure_count=failure_count+1,health_error=$2 WHERE id=$1 AND settings_revision=$3 AND COALESCE((SELECT sec.xmin::text||':'||sec.updated_at::text FROM secrets sec WHERE sec.proxy_id=proxies.id AND sec.id=proxies.secret_ref),'')=$4`, cfg.Proxy.ID, obsErr.Error(), cfg.ProxySettingsRevision, cfg.ProxySecretVersion)
	} else {
		result, err = s.DB.ExecContext(ctx, `UPDATE proxies SET status='healthy',exit_ip=$2,country=$3,country_code=lower($4),asn=$5,latency_ms=$6,last_checked_at=now(),last_success_at=now(),failure_count=0,consecutive_successes=1,health_error=NULL WHERE id=$1`+versionPredicate, cfg.Proxy.ID, obs.IP, obs.Country, obs.CountryCode, obs.ASN, obs.LatencyMS, cfg.ProxySettingsRevision, cfg.ProxySecretVersion)
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errProxyObservationStale
	}
	return nil
}
