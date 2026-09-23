package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

var ErrAccountDisabled = errors.New("account disabled")

type AccountConfig struct {
	ID             string
	Provider       string
	SecretRef      string
	Enabled        bool
	Network        network.Profile
	Proxy          *network.Proxy
	ProxySecretRef string
	ProxyUsername  string
}
type Repository struct{ DB *sql.DB }

func (r Repository) Account(ctx context.Context, id string) (AccountConfig, error) {
	var a AccountConfig
	var mode string
	var proxyID, proxyType, host, proxySecret, proxyUser, proxyStatus, proxyExit, proxyCountry, proxyASN sql.NullString
	var port sql.NullInt64
	err := r.DB.QueryRowContext(ctx, `SELECT a.id::text,a.provider,a.secret_ref,a.enabled,n.mode,COALESCE(n.proxy_id::text,''),COALESCE(p.type,''),COALESCE(p.host,''),COALESCE(p.port,0),COALESCE(p.secret_ref,''),COALESCE(p.username,''),COALESCE(p.status,'unknown'),COALESCE(p.exit_ip::text,''),COALESCE(p.country,''),COALESCE(p.asn,'') FROM accounts a JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id WHERE a.id=$1`, id).Scan(&a.ID, &a.Provider, &a.SecretRef, &a.Enabled, &mode, &proxyID, &proxyType, &host, &port, &proxySecret, &proxyUser, &proxyStatus, &proxyExit, &proxyCountry, &proxyASN)
	if err != nil {
		return a, err
	}
	if !a.Enabled {
		return a, ErrAccountDisabled
	}
	a.Network = network.Profile{AccountID: a.ID, Mode: network.RouteMode(mode)}
	if proxyID.String != "" {
		pid := proxyID.String
		a.Network.ProxyID = &pid
		p := network.Proxy{ID: pid, Type: network.ProxyType(proxyType.String), Host: host.String, Port: int(port.Int64), Status: network.ProxyStatus(proxyStatus.String), ExitIP: proxyExit.String, Country: proxyCountry.String, ASN: proxyASN.String}
		a.Proxy = &p
		a.ProxySecretRef = proxySecret.String
		a.ProxyUsername = proxyUser.String
	}
	return a, nil
}
