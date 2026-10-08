package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

var ErrIsolationWait = errors.New("account network isolation waiting for unique exit identity")

func fastGatewayExitIP(ctx context.Context, g *network.Gateway) (string, error) {
	urls := []string{"https://api.ipify.org?format=json", "https://api64.ipify.org?format=json"}
	if g == nil || g.Client == nil {
		return "", network.ErrProxyConfigInvalid
	}
	ctx, stop := context.WithTimeout(ctx, 4*time.Second)
	defer stop()
	var last error
	for _, u := range urls {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		// Cold proxy DNS + TLS must retain the original caller budget.
		probeCtx, cancel := context.WithCancel(ctx)
		req, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, u, nil)
		resp, err := g.Client.Do(req)
		if err == nil {
			var v struct {
				IP string `json:"ip"`
			}
			err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&v)
			resp.Body.Close()
			if err == nil && resp.StatusCode/100 == 2 {
				ip := net.ParseIP(strings.TrimSpace(v.IP))
				if ip != nil && ip.To4() != nil {
					cancel()
					return ip.To4().String(), nil
				}
			}
			if err == nil {
				err = errors.New("invalid exit ip")
			}
		}
		cancel()
		last = err
		// A second destination cannot repair rejected proxy credentials.
		if network.IsProxyAuthFailure(err) {
			return "", err
		}
	}
	return "", last
}

// The DB rollout controls the observer strategy as well as probe cadence.
// Disabling economy immediately restores the established parallel observer.
func (c Container) observeGatewayExitIP(ctx context.Context, g *network.Gateway) (string, error) {
	if g == nil {
		return "", network.ErrProxyConfigInvalid
	}
	enabled, err := c.Economy.Enabled(ctx, g.AccountID)
	if err != nil {
		return "", err
	}
	if !enabled {
		return parallelGatewayExitIP(ctx, g)
	}
	return fastGatewayExitIP(ctx, g)
}

func parallelGatewayExitIP(ctx context.Context, g *network.Gateway) (string, error) {
	urls := []string{"https://api.ipify.org?format=json", "https://api64.ipify.org?format=json"}
	type result struct {
		ip  string
		err error
	}
	ch := make(chan result, len(urls))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, u := range urls {
		go func(u string) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			resp, err := g.Client.Do(req)
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer resp.Body.Close()
			var v struct {
				IP string `json:"ip"`
			}
			if resp.StatusCode/100 != 2 || json.NewDecoder(resp.Body).Decode(&v) != nil || func() bool { ip := net.ParseIP(strings.TrimSpace(v.IP)); return ip == nil || ip.To4() == nil }() {
				ch <- result{err: errors.New("invalid exit ip")}
				return
			}
			ch <- result{ip: strings.TrimSpace(v.IP)}
		}(u)
	}
	var last error
	for range urls {
		r := <-ch
		if r.err == nil {
			cancel()
			return r.ip, nil
		}
		last = r.err
	}
	return "", last
}

// EnsureFreshNetworkIdentity is a fail-closed pre-mutation guard. It performs
// one fast proxied egress observation and atomically records READY/ISOLATION_WAIT.
func (c Container) EnsureFreshNetworkIdentity(ctx context.Context, accountID string) error {
	cfg, err := c.Accounts.Account(ctx, accountID)
	if errors.Is(err, ErrAccountDisabled) {
		var cleanup bool
		if qerr := c.DB.QueryRowContext(ctx, `SELECT deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1`, accountID).Scan(&cleanup); qerr == nil && cleanup {
			cfg, err = c.Accounts.AccountForCleanup(ctx, accountID)
		}
	}
	if err != nil && !errors.Is(err, ErrNetworkIdentityCollision) {
		return err
	}
	// A stale collision is allowed to reach the resolver so rotating proxies can recover.
	if errors.Is(err, ErrNetworkIdentityCollision) {
		var mode, proxyID, typ, host, user, ref, status, adapter, session, cc string
		var fallback bool
		var port int
		err = c.DB.QueryRowContext(ctx, `SELECT np.mode,p.id::text,p.type,p.host,p.port,COALESCE(p.username,''),COALESCE(p.secret_ref,''),p.status,COALESCE(p.adapter,'generic'),COALESCE(ani.sticky_session,''),COALESCE(ani.preferred_country_code,''),COALESCE(ani.fallback_active,false) FROM network_profiles np JOIN proxies p ON p.id=np.proxy_id LEFT JOIN account_network_identities ani ON ani.account_id=np.account_id WHERE np.account_id=$1`, accountID).Scan(&mode, &proxyID, &typ, &host, &port, &user, &ref, &status, &adapter, &session, &cc, &fallback)
		if err != nil {
			return err
		}
		cap := proxyAdapterByName(adapter).Capabilities()
		if cap.StickySession && session == "" {
			return ErrIsolationWait
		}
		if cap.StickySession {
			user = proxySessionUsername(adapter, user, cc, session, cap.CountryTargeting && !fallback && cc != "")
		}
		pid := proxyID
		cfg.ID = accountID
		cfg.Network = network.Profile{AccountID: accountID, Mode: network.RouteMode(mode), ProxyID: &pid}
		cfg.Proxy = &network.Proxy{ID: proxyID, Type: network.ProxyType(typ), Host: host, Port: port, Status: network.ProxyStatus(status)}
		cfg.ProxyUsername = user
		cfg.ProxyAdapter = adapter
		cfg.ProxySecretRef = ref
	}
	if cfg.Network.Mode != network.RouteProxyRequired {
		return nil
	}
	if cfg.Proxy == nil || cfg.Proxy.Status != network.StatusHealthy {
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
	g, err := network.NewAccountProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: cfg.ProxyUsername, Password: string(pass)}, "identity")
	if err != nil {
		return err
	}
	defer g.CloseIdleConnections()
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ip, err := c.observeGatewayExitIP(checkCtx, g)
	if err != nil {
		return err
	}
	// Verification must never rotate or overwrite identity. Sticky recovery is
	// the sole writer allowed to adopt a new exit IP/session. This guard only
	// admits the exact healthy identity already committed by that state machine.
	var expectedIP string
	var lastHealth bool
	var rotating bool
	err = c.DB.QueryRowContext(ctx, `SELECT COALESCE(host(exit_ip),''),COALESCE(last_health_ok,false),rotation_started_at IS NOT NULL FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&expectedIP, &lastHealth, &rotating)
	if err != nil {
		return err
	}
	if expectedIP == "" || !lastHealth || rotating || ip != expectedIP {
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail='proxy identity verification mismatch',runtime_status_at=now(),updated_at=now() WHERE id=$1 AND COALESCE(runtime_status,'') IN ('','READY','ISOLATION_WAIT','PROVIDER_PROXY_ERROR','PROVIDER_TRANSPORT_ERROR')`, accountID)
		return ErrIsolationWait
	}
	return nil
}
