package app

import (
	"context"
	"database/sql"
	"net"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

func (c Container) requireAccountNetworkReady(ctx context.Context, accountID string) error {
	var mode, proxyStatus, exitIP string
	var lastHealth sql.NullBool
	var rotationStarted sql.NullTime
	err := c.DB.QueryRowContext(ctx, `
SELECT np.mode,COALESCE(p.status,''),COALESCE(host(ani.exit_ip),''),
       ani.last_health_ok,ani.rotation_started_at
FROM network_profiles np
LEFT JOIN proxies p ON p.id=np.proxy_id
LEFT JOIN account_network_identities ani ON ani.account_id=np.account_id
WHERE np.account_id=$1
`, accountID).Scan(&mode, &proxyStatus, &exitIP, &lastHealth, &rotationStarted)
	if err != nil {
		return err
	}
	if mode != string(network.RouteProxyRequired) {
		return nil
	}
	if proxyStatus != string(network.StatusHealthy) {
		return ErrNetworkNotReady
	}
	ip := net.ParseIP(exitIP)
	if ip == nil || ip.To4() == nil {
		return ErrNetworkNotReady
	}
	if !lastHealth.Valid || !lastHealth.Bool || rotationStarted.Valid {
		return ErrNetworkNotReady
	}
	return nil
}
