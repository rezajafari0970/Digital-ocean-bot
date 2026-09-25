package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

const stickyFallbackAfter = 5 * time.Minute

type stickyGeo struct{ IP, Country, CountryCode, Timezone, ASN string }

func observeStickyGeo(ctx context.Context, g *network.Gateway) (stickyGeo, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/", nil)
	resp, err := g.Client.Do(req)
	if err != nil {
		return stickyGeo{}, err
	}
	defer resp.Body.Close()
	var raw struct {
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
		Connection struct {
			ASN int    `json:"asn"`
			Org string `json:"org"`
		} `json:"connection"`
	}
	if resp.StatusCode/100 != 2 || json.NewDecoder(resp.Body).Decode(&raw) != nil || net.ParseIP(strings.TrimSpace(raw.IP)) == nil {
		return stickyGeo{}, errors.New("invalid geo response")
	}
	asn := strings.TrimSpace(raw.Connection.Org)
	if raw.Connection.ASN > 0 {
		asn = fmt.Sprintf("AS%d %s", raw.Connection.ASN, asn)
	}
	return stickyGeo{IP: strings.TrimSpace(raw.IP), Country: raw.Country, CountryCode: strings.ToLower(raw.CountryCode), Timezone: strings.TrimSpace(raw.Timezone.ID), ASN: strings.TrimSpace(asn)}, nil
}

func (c Container) stickyConfig(ctx context.Context, accountID string) (AccountConfig, string, string, string, bool, *time.Time, error) {
	cfg, err := c.Accounts.Account(ctx, accountID)
	if errors.Is(err, ErrNetworkIdentityCollision) {
		err = nil
		cfg = AccountConfig{ID: accountID}
	}
	if err != nil {
		return cfg, "", "", "", false, nil, err
	}
	if cfg.Network.Mode != network.RouteProxyRequired {
		return cfg, "", "", "", false, nil, nil
	}
	var session, cc, country string
	var fallback bool
	var started *time.Time
	// STICKY_CAP_FROM_ACCOUNT_CONFIG_V1: use the same adapter value loaded by Repository.Account.
	// This removes the second adapter lookup and makes generic fail closed.
	stickyCap := proxyAdapterByName(cfg.ProxyAdapter).Capabilities().StickySession
	err = c.DB.QueryRowContext(ctx, `SELECT COALESCE(sticky_session,''),COALESCE(preferred_country_code,''),COALESCE(preferred_country,country,''),fallback_active,rotation_started_at FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&session, &cc, &country, &fallback, &started)
	if !stickyCap {
		_, err = c.DB.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,timezone,locale,sticky_session,fallback_active,rotation_started_at) VALUES($1,'UTC','en-US',NULL,false,NULL) ON CONFLICT(account_id) DO UPDATE SET sticky_session=NULL,fallback_active=false,rotation_started_at=NULL`, accountID)
		return cfg, "", cc, country, false, nil, err
	}
	if err != nil || session == "" {
		err = c.DB.QueryRowContext(ctx, `INSERT INTO account_network_identities(account_id,sticky_session,preferred_country,preferred_country_code) SELECT $1,replace(gen_random_uuid()::text,'-',''),COALESCE(p.country,''),CASE WHEN lower(COALESCE(p.country,''))='venezuela' THEN 've' ELSE '' END FROM network_profiles np JOIN proxies p ON p.id=np.proxy_id WHERE np.account_id=$1 ON CONFLICT(account_id) DO UPDATE SET sticky_session=COALESCE(account_network_identities.sticky_session,EXCLUDED.sticky_session),preferred_country=COALESCE(NULLIF(account_network_identities.preferred_country,''),EXCLUDED.preferred_country),preferred_country_code=COALESCE(NULLIF(account_network_identities.preferred_country_code,''),EXCLUDED.preferred_country_code) RETURNING sticky_session,COALESCE(preferred_country_code,''),COALESCE(preferred_country,''),fallback_active,rotation_started_at`, accountID).Scan(&session, &cc, &country, &fallback, &started)
	}
	return cfg, session, cc, country, fallback, started, err
}

// MaintainStickyIdentity keeps the current per-account sticky IP while healthy.
// Rotation starts only after failure. Preferred country gets five minutes of
// retries; only then may another country be accepted.
func (c Container) MaintainStickyIdentity(ctx context.Context, accountID string) error {

	// GENERIC_STATE_INVARIANT_V1
	// Generic adapters never own sticky/fallback/rotation state.
	var genericAdapter bool
	_ = c.DB.QueryRowContext(ctx, `
SELECT COALESCE(p.adapter,'generic')='generic'
FROM network_profiles np
JOIN proxies p ON p.id=np.proxy_id
WHERE np.account_id=$1
`, accountID).Scan(&genericAdapter)

	if genericAdapter {
		_, err := c.DB.ExecContext(ctx, `
UPDATE account_network_identities
SET sticky_session=NULL,
    fallback_active=false,
    rotation_started_at=NULL,
    updated_at=now()
WHERE account_id=$1
  AND (
      sticky_session IS NOT NULL
      OR fallback_active=true
      OR rotation_started_at IS NOT NULL
  )
`, accountID)
		if err != nil {
			return err
		}
	}

	cfg, session, cc, wanted, fallback, started, err := c.stickyConfig(ctx, accountID)
	if err != nil {
		return err
	}
	if cfg.Network.Mode != network.RouteProxyRequired {
		return nil
	}
	if cfg.Proxy == nil { // recover config without collision-sensitive repository path
		var typ, host, user, ref, status, adapter string
		var port int
		var pid string
		if err = c.DB.QueryRowContext(ctx, `SELECT p.id::text,p.type,p.host,p.port,COALESCE(p.username,''),COALESCE(p.secret_ref,''),p.status,COALESCE(p.adapter,'generic') FROM network_profiles np JOIN proxies p ON p.id=np.proxy_id WHERE np.account_id=$1`, accountID).Scan(&pid, &typ, &host, &port, &user, &ref, &status, &adapter); err != nil {
			return err
		}
		id := pid
		cfg.Network = network.Profile{AccountID: accountID, Mode: network.RouteProxyRequired, ProxyID: &id}
		cfg.Proxy = &network.Proxy{ID: pid, Type: network.ProxyType(typ), Host: host, Port: port, Status: network.ProxyStatus(status)}
		cfg.ProxyUsername = user
		cfg.ProxyAdapter = adapter
		cfg.ProxySecretRef = ref
	}
	if cfg.Proxy.Status != network.StatusHealthy {
		return ErrNetworkNotReady
	}
	var pass []byte
	if cfg.ProxySecretRef != "" {
		pass, err = c.Secrets.GetProxy(ctx, cfg.Proxy.ID, cfg.ProxySecretRef)
		if err != nil {
			return err
		}
		defer wipe(pass)
	}
	cap := proxyAdapterByName(cfg.ProxyAdapter).Capabilities()

	if !cap.StickySession {
		// GENERIC_RUNTIME_PATH_V1
		// Generic pools only observe health/geo. Never create sticky state.
		g, err := network.NewProxyGateway(
			accountID,
			*cfg.Proxy,
			network.ProxyCredentials{
				Username: cfg.ProxyUsername,
				Password: string(pass),
			},
		)
		if err != nil {
			return err
		}
		defer g.CloseIdleConnections()

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		geo, err := observeStickyGeo(probeCtx, g)
		if err != nil {
			_, _ = c.DB.ExecContext(ctx, `
				UPDATE account_network_identities
				SET last_health_at=now(),
				    last_health_ok=false,
				    sticky_session=NULL,
				    fallback_active=false,
				    rotation_started_at=NULL
				WHERE account_id=$1
			`, accountID)
			return err
		}

		var collision bool
		err = c.DB.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1
				FROM account_network_identities
				WHERE account_id<>$1
				AND (
					exit_ip=$2::inet
					OR subnet_key=(
						CASE
						WHEN family($2::inet)=4
						THEN host(network(set_masklen($2::inet,24)))||'/24'
						ELSE host(network(set_masklen($2::inet,48)))||'/48'
						END
					)
				)
			)
		`, accountID, geo.IP).Scan(&collision)

		if err != nil {
			return err
		}
		if collision {
			return ErrIsolationWait
		}

		newLocale := geoctx.LocaleForCountry(geo.CountryCode)

		_, err = c.DB.ExecContext(ctx, `
			UPDATE account_network_identities
			SET sticky_session=NULL,
			    exit_ip=$2::inet,
			    subnet_key=CASE
				    WHEN family($2::inet)=4
				    THEN host(network(set_masklen($2::inet,24)))||'/24'
				    ELSE host(network(set_masklen($2::inet,48)))||'/48'
			    END,
			    country=$3,
			    timezone=COALESCE(NULLIF($4,''),timezone),
			    locale=$5,
			    asn=$6,
			    last_health_at=now(),
			    last_health_ok=true,
			    rotation_started_at=NULL,
			    fallback_active=false,
			    updated_at=now()
			WHERE account_id=$1
		`,
			accountID,
			geo.IP,
			geo.Country,
			geo.Timezone,
			newLocale,
			geo.ASN,
		)

		return err
	}

	// First try the existing sticky session in the preferred country.
	targeted := cap.CountryTargeting && !fallback && cc != ""
	user := proxySessionUsername(cfg.ProxyAdapter, cfg.ProxyUsername, cc, session, targeted)
	g, err := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: user, Password: string(pass)})
	if err != nil {
		return err
	}
	defer g.CloseIdleConnections()
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ip, err := fastGatewayExitIP(checkCtx, g)
	var oldIP string
	_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(host(exit_ip),'') FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&oldIP)
	if err == nil && oldIP != "" && ip == oldIP && !fallback {
		_, _ = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET last_health_at=now(),last_health_ok=true,rotation_started_at=NULL,fallback_active=false,updated_at=updated_at WHERE account_id=$1`, accountID)
		return nil
	}
	if err == nil && oldIP != "" && ip == oldIP && fallback {
		// The temporary fallback is healthy. Keep serving through it, but probe
		// the preferred country in the background every keeper cycle. Switch
		// back only after a healthy, unique preferred-country candidate exists.
		_, _ = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET last_health_at=now(),last_health_ok=true WHERE account_id=$1`, accountID)
		if cc == "" {
			return nil
		}
		var preferredSession string
		if c.DB.QueryRowContext(ctx, `SELECT replace(gen_random_uuid()::text,'-','')`).Scan(&preferredSession) != nil {
			return nil
		}
		preferredUser := proxySessionUsername(cfg.ProxyAdapter, cfg.ProxyUsername, cc, preferredSession, cap.CountryTargeting)
		pg, pgErr := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: preferredUser, Password: string(pass)})
		if pgErr != nil {
			return nil
		}
		defer pg.CloseIdleConnections()
		probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
		defer probeCancel()
		preferredGeo, pgErr := observeStickyGeo(probeCtx, pg)
		if pgErr != nil || !strings.EqualFold(preferredGeo.CountryCode, cc) {
			return nil
		}
		var preferredCollision bool
		if c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND (exit_ip=$2::inet OR subnet_key=(CASE WHEN family($2::inet)=4 THEN host(network(set_masklen($2::inet,24)))||'/24' ELSE host(network(set_masklen($2::inet,48)))||'/48' END)))`, accountID, preferredGeo.IP).Scan(&preferredCollision) != nil || preferredCollision {
			return nil
		}
		var priorIP, priorCountry, priorTZ, priorLocale string
		_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(host(exit_ip),''),COALESCE(country,''),timezone,locale FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&priorIP, &priorCountry, &priorTZ, &priorLocale)
		newLocale := geoctx.LocaleForCountry(preferredGeo.CountryCode)
		_, pgErr = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET sticky_session=$2,exit_ip=$3::inet,subnet_key=CASE WHEN family($3::inet)=4 THEN host(network(set_masklen($3::inet,24)))||'/24' ELSE host(network(set_masklen($3::inet,48)))||'/48' END,country=$4,timezone=COALESCE(NULLIF($5,''),timezone),locale=$6,last_health_at=now(),last_health_ok=true,rotation_started_at=NULL,fallback_active=false,updated_at=now() WHERE account_id=$1`, accountID, preferredSession, preferredGeo.IP, preferredGeo.Country, preferredGeo.Timezone, newLocale)
		if pgErr == nil && (priorIP != preferredGeo.IP || priorCountry != preferredGeo.Country || priorTZ != preferredGeo.Timezone || priorLocale != newLocale) {
			c.queueBrowserAudit(ctx, accountID, "geo_changed")
		}
		if pgErr == nil {
			_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,runtime_status_at=now(),updated_at=now() WHERE id=$1`, accountID)
		}
		return pgErr
	}
	// Existing address disappeared or changed: begin/continue recovery.
	if started == nil {
		_, _ = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET rotation_started_at=now(),last_health_at=now(),last_health_ok=false WHERE account_id=$1`, accountID)
		now := time.Now()
		started = &now
	}
	allowFallback := fallback || (started != nil && time.Since(*started) >= stickyFallbackAfter)
	// A fresh session asks DataImpulse for another endpoint. Keep targeting the
	// preferred country during the five-minute recovery window.
	var newSession string
	if err = c.DB.QueryRowContext(ctx, `SELECT replace(gen_random_uuid()::text,'-','')`).Scan(&newSession); err != nil {
		return err
	}
	user = proxySessionUsername(cfg.ProxyAdapter, cfg.ProxyUsername, cc, newSession, cap.CountryTargeting && !allowFallback && cc != "")
	g2, err := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: user, Password: string(pass)})
	if err != nil {
		return err
	}
	defer g2.CloseIdleConnections()
	geoCtx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	geo, err := observeStickyGeo(geoCtx, g2)
	if err != nil {
		return err
	}
	if cap.CountryTargeting && !allowFallback && cc != "" && !strings.EqualFold(geo.CountryCode, cc) {
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail=$2,runtime_status_at=now() WHERE id=$1`, accountID, fmt.Sprintf("waiting for healthy %s proxy IP", wanted))
		return ErrIsolationWait
	}
	var collision bool
	err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND (exit_ip=$2::inet OR subnet_key=(CASE WHEN family($2::inet)=4 THEN host(network(set_masklen($2::inet,24)))||'/24' ELSE host(network(set_masklen($2::inet,48)))||'/48' END)))`, accountID, geo.IP).Scan(&collision)
	if err != nil {
		return err
	}
	if collision {
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail=$2,runtime_status_at=now() WHERE id=$1`, accountID, "waiting for unique proxy IP/subnet")
		return ErrIsolationWait
	}
	var priorCountry, priorTZ, priorLocale string
	_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(country,''),timezone,locale FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&priorCountry, &priorTZ, &priorLocale)
	newLocale := geoctx.LocaleForCountry(geo.CountryCode)
	_, err = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET sticky_session=$2,exit_ip=$3::inet,subnet_key=CASE WHEN family($3::inet)=4 THEN host(network(set_masklen($3::inet,24)))||'/24' ELSE host(network(set_masklen($3::inet,48)))||'/48' END,country=$4,timezone=COALESCE(NULLIF($5,''),timezone),locale=$6,last_health_at=now(),last_health_ok=true,rotation_started_at=NULL,fallback_active=$7,updated_at=now() WHERE account_id=$1`, accountID, newSession, geo.IP, geo.Country, geo.Timezone, newLocale, allowFallback)
	if err == nil && (priorCountry != geo.Country || priorTZ != geo.Timezone || priorLocale != newLocale) {
		c.queueBrowserAudit(ctx, accountID, "geo_changed")
	}
	if err != nil {
		return err
	}
	detail := ""
	if allowFallback && cc != "" && !strings.EqualFold(geo.CountryCode, cc) {
		detail = "preferred country unavailable for 5 minutes; temporary fallback: " + geo.Country
	}
	_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULLIF($2,''),runtime_status_at=now(),updated_at=now() WHERE id=$1`, accountID, detail)
	return nil
}

func (c Container) queueBrowserAudit(ctx context.Context, accountID, reason string) {
	_, _ = c.DB.ExecContext(ctx, `INSERT INTO browser_audit_queue(account_id,requested_at,not_before,reason,attempts,last_error) VALUES($1,now(),now()+interval '5 seconds',$2,0,NULL) ON CONFLICT(account_id) DO UPDATE SET requested_at=now(),not_before=now()+interval '5 seconds',reason=EXCLUDED.reason,attempts=0,last_error=NULL`, accountID, reason)
}
