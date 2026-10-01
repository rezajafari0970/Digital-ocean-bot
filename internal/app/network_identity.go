package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

var ErrIsolationWait = errors.New("account network isolation waiting for unique exit identity")

func fastGatewayExitIP(ctx context.Context, g *network.Gateway) (string, error) {
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
	g, err := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: cfg.ProxyUsername, Password: string(pass)})
	if err != nil {
		return err
	}
	defer g.CloseIdleConnections()
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ip, err := fastGatewayExitIP(checkCtx, g)
	if err != nil {
		return err
	}
	var collision bool
	err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND (exit_ip=$2::inet OR subnet_key=(host(network(set_masklen($2::inet,24)))||'/24')))`, accountID, ip).Scan(&collision)
	if err != nil {
		return err
	}
	if collision {
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail=$2,runtime_status_at=now(),updated_at=now() WHERE id=$1`, accountID, "proxy exit IP/subnet collision: "+ip)
		return ErrIsolationWait
	}
	_, err = c.DB.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,exit_ip,subnet_key) VALUES($1,$2::inet,host(network(set_masklen($2::inet,24)))||'/24') ON CONFLICT(account_id) DO UPDATE SET country=CASE WHEN account_network_identities.exit_ip=EXCLUDED.exit_ip THEN account_network_identities.country ELSE NULL END,country_code=CASE WHEN account_network_identities.exit_ip=EXCLUDED.exit_ip THEN account_network_identities.country_code ELSE NULL END,asn=CASE WHEN account_network_identities.exit_ip=EXCLUDED.exit_ip THEN account_network_identities.asn ELSE NULL END,exit_ip=EXCLUDED.exit_ip,subnet_key=EXCLUDED.subnet_key,updated_at=now()`, accountID, ip)
	if err != nil {
		return err
	}
	_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,runtime_status_at=now(),updated_at=now() WHERE id=$1`, accountID)
	return nil
}
