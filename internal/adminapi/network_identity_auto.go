package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net"
	"net/http"
	"strings"
	"time"
)

func fastExitIP(ctx context.Context, g *network.Gateway) (string, error) {
	endpoints := []string{"https://api.ipify.org?format=json", "https://api64.ipify.org?format=json"}
	type result struct {
		ip  string
		err error
	}
	ch := make(chan result, len(endpoints))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, u := range endpoints {
		go func(url string) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := g.Client.Do(req)
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer resp.Body.Close()
			var v struct {
				IP string `json:"ip"`
			}
			if resp.StatusCode/100 != 2 || json.NewDecoder(resp.Body).Decode(&v) != nil || net.ParseIP(strings.TrimSpace(v.IP)) == nil {
				ch <- result{err: errors.New("invalid exit ip")}
				return
			}
			ch <- result{ip: strings.TrimSpace(v.IP)}
		}(u)
	}
	var last error
	for range endpoints {
		r := <-ch
		if r.err == nil {
			cancel()
			return r.ip, nil
		}
		last = r.err
	}
	return "", last
}

func (s *Server) refreshNetworkIdentityIfDue(ctx context.Context, accountID string) {
	var proxyID, name, typ, host, username, secretRef string
	var port int
	var last sql.NullTime
	err := s.DB.QueryRowContext(ctx, "SELECT p.id::text,p.name,p.type,p.host,p.port,COALESCE(p.username,''),COALESCE(p.secret_ref,''),ani.updated_at FROM network_profiles np JOIN proxies p ON p.id=np.proxy_id LEFT JOIN account_network_identities ani ON ani.account_id=np.account_id WHERE np.account_id=$1 AND np.mode='proxy_required'", accountID).Scan(&proxyID, &name, &typ, &host, &port, &username, &secretRef, &last)
	if err != nil || (last.Valid && time.Since(last.Time) < 15*time.Second) {
		return
	}
	var password []byte
	if secretRef != "" {
		password, err = s.Container.Secrets.GetProxy(ctx, proxyID, secretRef)
		if err != nil {
			return
		}
		defer zeroBytes(password)
	}
	p := network.Proxy{ID: proxyID, Name: name, Type: network.ProxyType(typ), Host: host, Port: port, Status: network.StatusHealthy}
	g, err := network.NewProxyGateway(accountID, p, network.ProxyCredentials{Username: username, Password: string(password)})
	if err != nil {
		return
	}
	defer g.CloseIdleConnections()
	fastCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ip, err := fastExitIP(fastCtx, g)
	if err != nil {
		return
	}
	var collision bool
	_ = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND (exit_ip=$2::inet OR subnet_key=(CASE WHEN family($2::inet)=4 THEN host(network(set_masklen($2::inet,24)))||'/24' ELSE host(network(set_masklen($2::inet,48)))||'/48' END)))`, accountID, ip).Scan(&collision)
	if collision {
		_, _ = s.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='ISOLATION_WAIT',runtime_status_detail=$2,runtime_status_at=now(),updated_at=now() WHERE id=$1`, accountID, "proxy exit IP/subnet collision: "+ip)
		return
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,exit_ip,subnet_key) VALUES($1,$2::inet,CASE WHEN family($2::inet)=4 THEN host(network(set_masklen($2::inet,24)))||'/24' ELSE host(network(set_masklen($2::inet,48)))||'/48' END) ON CONFLICT(account_id) DO UPDATE SET exit_ip=EXCLUDED.exit_ip,subnet_key=EXCLUDED.subnet_key,updated_at=now()`, accountID, ip)
	_, _ = s.DB.ExecContext(ctx, `UPDATE proxies SET exit_ip=$2,last_checked_at=now(),last_success_at=now(),status='healthy',failure_count=0 WHERE id=$1`, proxyID, ip)
	_, _ = s.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='READY',runtime_status_detail=NULL,runtime_status_at=now(),updated_at=now() WHERE id=$1 AND runtime_status='ISOLATION_WAIT'`, accountID)
}
