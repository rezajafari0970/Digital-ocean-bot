package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

var (
	ErrAccountDisabled          = errors.New("account disabled")
	ErrNetworkIdentityCollision = errors.New("account network identity collision")
)

type AccountConfig struct {
	ID             string
	Provider       string
	SecretRef      string
	Enabled        bool
	Network        network.Profile
	Proxy          *network.Proxy
	ProxySecretRef string
	ProxyUsername  string
	ProxyAdapter   string
}
type Repository struct{ DB *sql.DB }

func (r Repository) Account(ctx context.Context, id string) (AccountConfig, error) {
	var a AccountConfig
	var mode string
	var proxyID, proxyType, host, proxySecret, proxyUser, proxyStatus, proxyExit, proxyCountry, proxyASN sql.NullString
	var port sql.NullInt64
	err := r.DB.QueryRowContext(ctx, `SELECT a.id::text,a.provider,a.secret_ref,a.enabled,n.mode,COALESCE(n.proxy_id::text,''),COALESCE(p.type,''),COALESCE(p.host,''),COALESCE(p.port,0),COALESCE(p.secret_ref,''),COALESCE(p.username,''),COALESCE(p.status,'unknown'),COALESCE(p.exit_ip::text,''),COALESCE(p.country,''),COALESCE(p.asn,''),COALESCE(p.adapter,'generic') FROM accounts a JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id WHERE a.id=$1`, id).Scan(&a.ID, &a.Provider, &a.SecretRef, &a.Enabled, &mode, &proxyID, &proxyType, &host, &port, &proxySecret, &proxyUser, &proxyStatus, &proxyExit, &proxyCountry, &proxyASN, &a.ProxyAdapter)
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
		if a.Network.Mode == network.RouteProxyRequired {
			var collision bool
			err = r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities me JOIN account_network_identities other_i ON other_i.account_id<>me.account_id JOIN network_profiles other_np ON other_np.account_id=other_i.account_id AND other_np.mode='proxy_required' WHERE me.account_id=$1 AND me.exit_ip IS NOT NULL AND (other_i.exit_ip=me.exit_ip OR (me.subnet_key<>'' AND other_i.subnet_key=me.subnet_key)))`, id).Scan(&collision)
			if err != nil {
				return a, err
			}
			if collision {
				return a, ErrNetworkIdentityCollision
			}
		}
		p := network.Proxy{ID: pid, Type: network.ProxyType(proxyType.String), Host: host.String, Port: int(port.Int64), Status: network.ProxyStatus(proxyStatus.String), ExitIP: proxyExit.String, Country: proxyCountry.String, ASN: proxyASN.String}
		a.Proxy = &p
		a.ProxySecretRef = proxySecret.String
		a.ProxyUsername = proxyUser.String
		if a.Network.Mode == network.RouteProxyRequired {
			var session, cc string
			var fallback bool
			if qerr := r.DB.QueryRowContext(ctx, `SELECT COALESCE(sticky_session,''),COALESCE(preferred_country_code,''),fallback_active FROM account_network_identities WHERE account_id=$1`, id).Scan(&session, &cc, &fallback); qerr == nil && session != "" && proxyAdapterByName(a.ProxyAdapter).Capabilities().StickySession {
				a.ProxyUsername = proxySessionUsername(a.ProxyAdapter, proxyUser.String, cc, session, !fallback && cc != "")
			}
		}
	}
	return a, nil
}
