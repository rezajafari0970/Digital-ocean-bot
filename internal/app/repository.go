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
	ID                            string
	Provider                      string
	SecretRef                     string
	Enabled                       bool
	Network                       network.Profile
	Proxy                         *network.Proxy
	ProxySecretRef                string
	ProxyUsername                 string
	ProxyAdapter                  string
	ProxyBaseUsername             string
	ProxySecretVersion            string
	ProxyRouteFingerprint         string
	ProxySettingsRevision         int64
	NetworkSession, NetworkExitIP string
}

// routeFingerprintSQL uses aliases n, p and i in the same MVCC snapshot as credentials.
const routeFingerprintSQL = `md5(jsonb_build_array(n.mode,n.proxy_id,p.type,p.host,p.port,p.username,p.adapter,p.secret_ref,COALESCE((SELECT sec.xmin::text||':'||sec.updated_at::text FROM secrets sec WHERE sec.proxy_id=p.id AND sec.id=p.secret_ref),''),i.sticky_session,i.sticky_port,i.selected_target_country,i.preferred_country_code,i.fallback_active,host(i.exit_ip))::text)`

type Repository struct{ DB *sql.DB }

func (r Repository) Account(ctx context.Context, id string) (AccountConfig, error) {
	return r.account(ctx, id, false)
}

func (r Repository) AccountForCleanup(ctx context.Context, id string) (AccountConfig, error) {
	return r.account(ctx, id, true)
}

func (r Repository) account(ctx context.Context, id string, allowDisabled bool) (AccountConfig, error) {
	var a AccountConfig
	var mode string
	var proxyID, proxyType, host, proxySecret, proxyUser, proxyStatus, proxyExit, proxyCountry, proxyCountryCode, proxyASN sql.NullString
	var port sql.NullInt64
	var cc string
	var fallback bool
	var stickyPort int
	err := r.DB.QueryRowContext(ctx, `SELECT a.id::text,a.provider,a.secret_ref,a.enabled,n.mode,COALESCE(n.proxy_id::text,''),COALESCE(p.type,''),COALESCE(p.host,''),COALESCE(p.port,0),COALESCE(p.secret_ref,''),COALESCE(p.username,''),COALESCE(p.status,'unknown'),COALESCE(p.exit_ip::text,''),COALESCE(p.country,''),COALESCE(p.country_code,''),COALESCE(p.asn,''),COALESCE(p.adapter,'generic'),COALESCE((SELECT sec.xmin::text||':'||sec.updated_at::text FROM secrets sec WHERE sec.proxy_id=p.id AND sec.id=p.secret_ref),''),COALESCE(p.settings_revision,0),COALESCE(i.sticky_session,''),COALESCE(host(i.exit_ip),''),COALESCE(i.selected_target_country,i.preferred_country_code,''),CASE WHEN i.selected_target_country IS NOT NULL THEN false ELSE COALESCE(i.fallback_active,false) END,COALESCE(i.sticky_port,0),`+routeFingerprintSQL+` FROM accounts a JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id LEFT JOIN account_network_identities i ON i.account_id=a.id WHERE a.id=$1`, id).Scan(&a.ID, &a.Provider, &a.SecretRef, &a.Enabled, &mode, &proxyID, &proxyType, &host, &port, &proxySecret, &proxyUser, &proxyStatus, &proxyExit, &proxyCountry, &proxyCountryCode, &proxyASN, &a.ProxyAdapter, &a.ProxySecretVersion, &a.ProxySettingsRevision, &a.NetworkSession, &a.NetworkExitIP, &cc, &fallback, &stickyPort, &a.ProxyRouteFingerprint)
	if err != nil {
		return a, err
	}
	if !a.Enabled && !allowDisabled {
		return a, ErrAccountDisabled
	}
	a.Network = network.Profile{AccountID: a.ID, Mode: network.RouteMode(mode)}
	if proxyID.String != "" {
		pid := proxyID.String
		a.Network.ProxyID = &pid
		p := network.Proxy{ID: pid, Type: network.ProxyType(proxyType.String), Host: host.String, Port: int(port.Int64), Status: network.ProxyStatus(proxyStatus.String), ExitIP: proxyExit.String, Country: proxyCountry.String, CountryCode: proxyCountryCode.String, ASN: proxyASN.String}
		a.Proxy = &p
		a.ProxySecretRef = proxySecret.String
		a.ProxyUsername = proxyUser.String
		a.ProxyBaseUsername = proxyUser.String
		if a.Network.Mode == network.RouteProxyRequired {
			if stickyPort >= 10000 && stickyPort <= 20000 && proxyAdapterByName(a.ProxyAdapter).Capabilities().DedicatedPort {
				a.Proxy.Port = stickyPort
			}
			if a.NetworkSession != "" && proxyAdapterByName(a.ProxyAdapter).Capabilities().StickySession {
				a.ProxyUsername = proxySessionUsername(a.ProxyAdapter, proxyUser.String, cc, a.NetworkSession, !fallback && cc != "" && proxyAdapterByName(a.ProxyAdapter).Capabilities().CountryTargeting)
			}
		}
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

	}
	return a, nil
}
