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

const (
	stickySameIPRetryFor = 2 * time.Minute
	stickyFallbackAfter  = 5 * time.Minute
)

var ErrProxyObservationUnavailable = errors.New("proxy identity observation unavailable")

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
	if resp.StatusCode/100 != 2 || json.NewDecoder(resp.Body).Decode(&raw) != nil || func() bool { ip := net.ParseIP(strings.TrimSpace(raw.IP)); return ip == nil || ip.To4() == nil }() {
		return stickyGeo{}, errors.New("invalid geo response")
	}
	asn := strings.TrimSpace(raw.Connection.Org)
	if raw.Connection.ASN > 0 {
		asn = fmt.Sprintf("AS%d %s", raw.Connection.ASN, asn)
	}
	return stickyGeo{IP: strings.TrimSpace(raw.IP), Country: raw.Country, CountryCode: strings.ToLower(raw.CountryCode), Timezone: strings.TrimSpace(raw.Timezone.ID), ASN: strings.TrimSpace(asn)}, nil
}

func observeStickyGeoReliable(ctx context.Context, g *network.Gateway) (stickyGeo, error) {
	geo, err := observeStickyGeo(ctx, g)
	if err == nil {
		return geo, nil
	}
	if _, probeErr := fastGatewayExitIP(ctx, g); probeErr == nil {
		return stickyGeo{}, ErrProxyObservationUnavailable
	} else {
		return stickyGeo{}, probeErr
	}
}

func (c Container) stickyConfig(ctx context.Context, accountID string) (AccountConfig, string, string, string, bool, *time.Time, error) {
	cfg, err := c.Accounts.Account(ctx, accountID)
	if errors.Is(err, ErrNetworkIdentityCollision) {
		// Repository already populated the account/network fields before the
		// collision check. Keep that partial config so the proxy keeper can
		// repair the collision instead of silently opting out of recovery.
		err = nil
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
	if stickyCap {
		stickyPort, perr := c.ensureAccountStickyPort(ctx, accountID)
		if perr != nil {
			return cfg, "", "", "", false, nil, perr
		}
		if cfg.Proxy != nil {
			cfg.Proxy.Port = stickyPort
		}
	}
	err = c.DB.QueryRowContext(ctx, `SELECT COALESCE(sticky_session,''),COALESCE(preferred_country_code,''),COALESCE(preferred_country,country,''),fallback_active,rotation_started_at FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&session, &cc, &country, &fallback, &started)
	if !stickyCap {
		_, err = c.DB.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,timezone,locale,sticky_session,fallback_active,rotation_started_at) VALUES($1,'UTC','en-US',NULL,false,NULL) ON CONFLICT(account_id) DO UPDATE SET sticky_session=NULL,fallback_active=false,rotation_started_at=NULL`, accountID)
		return cfg, "", cc, country, false, nil, err
	}
	if err != nil || session == "" {
		err = c.DB.QueryRowContext(ctx, `INSERT INTO account_network_identities(account_id,sticky_session,preferred_country,preferred_country_code) SELECT $1,replace(gen_random_uuid()::text,'-',''),COALESCE(p.country,''),lower(COALESCE(p.country_code,'')) FROM network_profiles np JOIN proxies p ON p.id=np.proxy_id WHERE np.account_id=$1 ON CONFLICT(account_id) DO UPDATE SET sticky_session=COALESCE(account_network_identities.sticky_session,EXCLUDED.sticky_session),preferred_country=COALESCE(NULLIF(account_network_identities.preferred_country,''),EXCLUDED.preferred_country),preferred_country_code=COALESCE(NULLIF(account_network_identities.preferred_country_code,''),EXCLUDED.preferred_country_code) RETURNING sticky_session,COALESCE(preferred_country_code,''),COALESCE(preferred_country,''),fallback_active,rotation_started_at`, accountID).Scan(&session, &cc, &country, &fallback, &started)
	}
	if err == nil && cc == "" {
		var proxyCode, proxyCountry string
		if qerr := c.DB.QueryRowContext(ctx, `
SELECT lower(COALESCE(p.country_code,'')),COALESCE(p.country,'')
FROM network_profiles np
JOIN proxies p ON p.id=np.proxy_id
WHERE np.account_id=$1
`, accountID).Scan(&proxyCode, &proxyCountry); qerr == nil {
			if proxyCode == "" {
				proxyCode = geoctx.CountryCodeForName(proxyCountry)
			}
			if proxyCode != "" {
				cc = strings.ToLower(proxyCode)
				_, _ = c.DB.ExecContext(ctx, `
UPDATE account_network_identities
SET preferred_country_code=$2,
    preferred_country=COALESCE(NULLIF(preferred_country,''),NULLIF($3,'')),
    updated_at=now()
WHERE account_id=$1
`, accountID, cc, proxyCountry)
				if country == "" {
					country = proxyCountry
				}
			}
		}
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

	cfg, session, cc, _, fallback, started, err := c.stickyConfig(ctx, accountID)
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

		geo, err := observeStickyGeoReliable(probeCtx, g)
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
					OR subnet_key=host(network(set_masklen($2::inet,24)))||'/24'
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
			    subnet_key=host(network(set_masklen($2::inet,24)))||'/24',
			    country=$3,
			    country_code=lower($4),
			    timezone=COALESCE(NULLIF($5,''),timezone),
			    locale=$6,
			    asn=$7,
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
			geo.CountryCode,
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
		preferredGeo, pgErr := observeStickyGeoReliable(probeCtx, pg)
		if pgErr != nil || !strings.EqualFold(preferredGeo.CountryCode, cc) {
			return nil
		}
		var preferredCollision bool
		if c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND (exit_ip=$2::inet OR subnet_key=(host(network(set_masklen($2::inet,24)))||'/24')))`, accountID, preferredGeo.IP).Scan(&preferredCollision) != nil || preferredCollision {
			return nil
		}
		var priorIP, priorCountry, priorTZ, priorLocale string
		_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(host(exit_ip),''),COALESCE(country,''),timezone,locale FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&priorIP, &priorCountry, &priorTZ, &priorLocale)
		newLocale := geoctx.LocaleForCountry(preferredGeo.CountryCode)
		_, pgErr = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET sticky_session=$2,exit_ip=$3::inet,subnet_key=host(network(set_masklen($3::inet,24)))||'/24',country=$4,country_code=lower($5),timezone=COALESCE(NULLIF($6,''),timezone),locale=$7,last_health_at=now(),last_health_ok=true,rotation_started_at=NULL,fallback_active=false,updated_at=now() WHERE account_id=$1`, accountID, preferredSession, preferredGeo.IP, preferredGeo.Country, preferredGeo.CountryCode, preferredGeo.Timezone, newLocale)
		if pgErr == nil && (priorIP != preferredGeo.IP || priorCountry != preferredGeo.Country || priorTZ != preferredGeo.Timezone || priorLocale != newLocale) {
		}
		if pgErr == nil {
			_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,provider_error_state=CASE WHEN provider_error_state='TRANSPORT_ERROR' AND COALESCE(provider_error_detail,'') LIKE '%account network not ready%' THEN NULL ELSE provider_error_state END,provider_error_detail=CASE WHEN provider_error_state='TRANSPORT_ERROR' AND COALESCE(provider_error_detail,'') LIKE '%account network not ready%' THEN NULL ELSE provider_error_detail END,runtime_status_at=now(),updated_at=now() WHERE id=$1 AND provider_state='ACTIVE' AND (COALESCE(provider_error_state,'')='' OR (provider_error_state='TRANSPORT_ERROR' AND COALESCE(provider_error_detail,'') LIKE '%account network not ready%'))`, accountID)
		}
		return pgErr
	}
	// Existing address disappeared or changed: begin/continue recovery.
	if started == nil {
		_, _ = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET rotation_started_at=now(),last_health_at=now(),last_health_ok=false WHERE account_id=$1`, accountID)
		now := time.Now()
		started = &now
	}
	if started != nil && retryPreviousProxyIP(oldIP, *started, time.Now()) {
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail='retrying previous proxy IPv4',runtime_status_at=now(),updated_at=now() WHERE id=$1`, accountID)
		return ErrIsolationWait
	}
	allowFallback := fallback
	if started != nil {
		allowFallback = allowCrossCountryFallback(fallback, *started, time.Now())
	}
	// Search several independent sessions in the same keeper cycle. Residential
	// pools can legitimately return collisions; do not stall the whole account
	// for another ten seconds after the first duplicate /24.
	const candidateAttempts = 6
	var newSession string
	var geo stickyGeo
	var candidateErr error
	found := false
	for attempt := 0; attempt < candidateAttempts; attempt++ {
		if err = c.DB.QueryRowContext(ctx, `SELECT replace(gen_random_uuid()::text,'-','')`).Scan(&newSession); err != nil {
			return err
		}
		user = proxySessionUsername(cfg.ProxyAdapter, cfg.ProxyUsername, cc, newSession, cap.CountryTargeting && !allowFallback && cc != "")
		g2, gerr := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: user, Password: string(pass)})
		if gerr != nil {
			candidateErr = gerr
			continue
		}
		geoCtx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
		candidate, gerr := observeStickyGeoReliable(geoCtx, g2)
		cancel2()
		if gerr != nil {
			g2.CloseIdleConnections()
			candidateErr = gerr
			continue
		}
		if cap.CountryTargeting && !allowFallback && cc != "" && !strings.EqualFold(candidate.CountryCode, cc) {
			candidateErr = ErrIsolationWait
			continue
		}
		var collision bool
		if qerr := c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND (exit_ip=$2::inet OR subnet_key=(host(network(set_masklen($2::inet,24)))||'/24')))`, accountID, candidate.IP).Scan(&collision); qerr != nil {
			return qerr
		}
		if collision {
			candidateErr = ErrIsolationWait
			continue
		}
		// Prove sticky persistence across fresh proxy connections. Reusing g2
		// would only prove HTTP keep-alive stability, not that the provider maps
		// this session to the same exit after reconnect.
		g2.CloseIdleConnections()
		stable := true
		for verify := 0; verify < 2; verify++ {
			vg, verr := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: user, Password: string(pass)})
			if verr != nil {
				stable = false
				candidateErr = verr
				break
			}
			verifyCtx, verifyCancel := context.WithTimeout(ctx, 4*time.Second)
			verifyIP, verr := fastGatewayExitIP(verifyCtx, vg)
			verifyCancel()
			vg.CloseIdleConnections()
			if verr != nil || verifyIP != candidate.IP {
				stable = false
				candidateErr = ErrIsolationWait
				break
			}
		}
		if !stable {
			continue
		}
		geo = candidate
		found = true
		break
	}
	if !found {
		detail := fmt.Sprintf("no unique proxy IPv4/subnet after %d session candidates", candidateAttempts)
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail=$2,runtime_status_at=now() WHERE id=$1`, accountID, detail)
		if candidateErr != nil && !errors.Is(candidateErr, ErrIsolationWait) {
			return candidateErr
		}
		return ErrIsolationWait
	}
	var priorCountry, priorTZ, priorLocale string
	_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(country,''),timezone,locale FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&priorCountry, &priorTZ, &priorLocale)
	newLocale := geoctx.LocaleForCountry(geo.CountryCode)
	_, err = c.DB.ExecContext(ctx, `UPDATE account_network_identities SET sticky_session=$2,exit_ip=$3::inet,subnet_key=host(network(set_masklen($3::inet,24)))||'/24',country=$4,country_code=lower($5),timezone=COALESCE(NULLIF($6,''),timezone),locale=$7,last_health_at=now(),last_health_ok=true,rotation_started_at=NULL,fallback_active=$8,updated_at=now() WHERE account_id=$1`, accountID, newSession, geo.IP, geo.Country, geo.CountryCode, geo.Timezone, newLocale, allowFallback)
	if err == nil && (priorCountry != geo.Country || priorTZ != geo.Timezone || priorLocale != newLocale) {
	}
	if err != nil {
		return classifyIdentityWriteError(err)
	}
	detail := ""
	if allowFallback && cc != "" && !strings.EqualFold(geo.CountryCode, cc) {
		detail = "preferred country unavailable for 5 minutes; temporary fallback: " + geo.Country
	}
	_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULLIF($2,''),provider_error_state=CASE WHEN provider_error_state='TRANSPORT_ERROR' AND COALESCE(provider_error_detail,'') LIKE '%account network not ready%' THEN NULL ELSE provider_error_state END,provider_error_detail=CASE WHEN provider_error_state='TRANSPORT_ERROR' AND COALESCE(provider_error_detail,'') LIKE '%account network not ready%' THEN NULL ELSE provider_error_detail END,runtime_status_at=now(),updated_at=now() WHERE id=$1 AND provider_state='ACTIVE' AND (COALESCE(provider_error_state,'')='' OR (provider_error_state='TRANSPORT_ERROR' AND COALESCE(provider_error_detail,'') LIKE '%account network not ready%'))`, accountID, detail)
	return nil
}
