package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

// OnboardingClient is the provider-independent boundary for discovery and
// authorization before an account exists. Explicit choices never fall back.
// Proxy definitions and credential versions remain locked until Close.
func (c Container) OnboardingClient(ctx context.Context, proxyID, country string) (*http.Client, func(), string, error) {
	return c.onboardingClient(ctx, proxyID, country, func(_ context.Context, id string, p network.Proxy, credentials network.ProxyCredentials) (*network.Gateway, error) {
		return network.NewProxyGateway(id, p, credentials)
	})
}

func (c Container) onboardingClient(ctx context.Context, proxyID, country string, gatewayFor countryGatewayFactory, capture ...*OnboardingRoute) (*http.Client, func(), string, error) {
	strict := len(capture) > 0 && capture[0].StrictIdentity
	country, err := proxyselection.Country(country)
	if err != nil {
		return nil, nil, "", errors.New("invalid_proxy_country")
	}
	if (country != proxyselection.Auto || strict) && proxyID == "" {
		return nil, nil, "", errors.New("proxy_country_requires_proxy")
	}
	session, err := countrySession()
	if err != nil {
		return nil, nil, "", err
	}
	// No shared preview session: separate drafts cannot reuse another identity.
	identity := session[:8] + "-" + session[8:12] + "-4" + session[13:16] + "-8" + session[17:20] + "-" + session[20:]
	if proxyID == "" {
		b, err := network.NewIsolatedDirectClient(identity)
		if err != nil {
			return nil, nil, "", err
		}
		if len(capture) > 0 {
			capture[0].Client = b.Client
			capture[0].Close = b.CloseIdleConnections
			capture[0].Country = proxyselection.Auto
		}
		return b.Client, b.CloseIdleConnections, "", nil
	}
	if c.DB == nil {
		return nil, nil, "", ErrNetworkNotReady
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, "", err
	}
	owned := false
	defer func() {
		if !owned {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout='3s'`); err != nil {
		return nil, nil, "", err
	}
	var p countryProxy
	err = tx.QueryRowContext(ctx, `SELECT id::text,type,host,port,COALESCE(username,''),adapter,COALESCE(secret_ref,''),status FROM proxies WHERE id=$1 FOR SHARE`, proxyID).Scan(&p.ID, &p.Type, &p.Host, &p.Port, &p.Username, &p.Adapter, &p.SecretRef, &p.Status)
	if err != nil {
		return nil, nil, "", err
	}
	if p.Status != "healthy" {
		return nil, nil, "", unusableOnboarding("proxy_not_healthy")
	}
	if !ProxyAdapterRegistered(p.Adapter) {
		return nil, nil, "", unusableOnboarding("proxy_driver_unavailable")
	}
	if p.SecretRef != "" {
		err = tx.QueryRowContext(ctx, `SELECT xmin::text||':'||updated_at::text FROM secrets WHERE proxy_id=$1 AND id=$2 FOR SHARE`, p.ID, p.SecretRef).Scan(&p.SecretVersion)
		if err != nil || c.Secrets == nil {
			return nil, nil, "", errors.New("proxy_credentials_unavailable")
		}
	}
	cap := ProxyAdapterCapabilities(p.Adapter)
	if (country != proxyselection.Auto || strict) && !cap.StickySession && !cap.FixedEgress {
		return nil, nil, "", unusableOnboarding("proxy_country_stable_driver_required")
	}
	v := countryCandidate{ProxyID: p.ID, Session: session, Port: p.Port}
	snapshot := countrySnapshot{Config: countryConfig{Account: identity}, Stage: proxyselection.Auto}
	if country != proxyselection.Auto || strict {
		if country != proxyselection.Auto {
			v.Target = country
		}
		var evaluation proxyselection.Evaluation
		v, evaluation = c.verifyCountryCandidateUsing(ctx, snapshot, p, v, gatewayFor, tx, func(ctx context.Context, id, ref, version string) ([]byte, error) {
			return c.Secrets.GetProxyAtVersionTx(ctx, tx, id, ref, version)
		})
		if evaluation.Health != proxyselection.Usable {
			return nil, nil, "", &onboardingSelectionError{evaluation}
		}
	}
	var password []byte
	if p.SecretRef != "" {
		password, err = c.Secrets.GetProxyAtVersionTx(ctx, tx, p.ID, p.SecretRef, p.SecretVersion)
		if err != nil {
			return nil, nil, "", errors.New("proxy_credentials_unavailable")
		}
		defer wipe(password)
	}
	user := p.Username
	if cap.StickySession {
		user = ProxySessionUsername(p.Adapter, user, v.Target, v.Session, v.Target != "" && cap.CountryTargeting)
	}
	proxy := network.Proxy{ID: p.ID, Type: network.ProxyType(p.Type), Host: p.Host, Port: v.Port, Status: network.StatusHealthy}
	credentials := network.ProxyCredentials{Username: user, Password: string(password)}
	gateway, err := gatewayFor(ctx, identity, proxy, credentials)
	if err != nil {
		return nil, nil, "", errors.New("proxy_not_ready")
	}
	var closed atomic.Bool
	var closeOnce sync.Once
	closeFn := func() { closeOnce.Do(func() { closed.Store(true); gateway.CloseIdleConnections(); _ = tx.Rollback() }) }
	check := func(checkCtx context.Context) error {
		if closed.Load() || ctx.Err() != nil {
			return ErrStaleAccountNetwork
		}
		var alive int
		if err := tx.QueryRowContext(checkCtx, `SELECT 1`).Scan(&alive); err != nil {
			return ErrStaleAccountNetwork
		}
		if v.Target == "" && !strict {
			return nil
		}
		// Reobserve through a fresh connection before each provider dispatch.
		// A changed/missing country or IP blocks that request, including redirects.
		probeCtx, cancel := context.WithTimeout(checkCtx, 7*time.Second)
		defer cancel()
		probe, err := gatewayFor(probeCtx, identity, proxy, credentials)
		if err != nil {
			return ErrProxyObservationUnavailable
		}
		geo, err := observeStickyCandidate(probeCtx, probe)
		if err != nil || geo.IP != v.Geo.IP || !strings.EqualFold(geo.CountryCode, v.Geo.CountryCode) {
			return ErrProxyObservationUnavailable
		}
		var collision bool
		if err = tx.QueryRowContext(checkCtx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE exit_ip=$1::inet OR subnet_key=host(network(set_masklen($1::inet,24)))||'/24')`, geo.IP).Scan(&collision); err != nil {
			return err
		}
		if collision {
			return ErrNetworkIdentityCollision
		}
		return nil
	}
	client := *gateway.Client
	client.Transport = accountNetworkGuardTransport{Base: gateway.Client.Transport, Check: check, NativeWriteTrace: true}
	if len(capture) > 0 {
		route := capture[0]
		route.Client = &client
		route.Close = closeFn
		route.Check = check
		route.ProxyID = p.ID
		route.Country = country
		route.IP = v.Geo.IP
		route.Timezone = v.Geo.Timezone
		if strict {
			route.Country = strings.ToUpper(v.Geo.CountryCode)
		}
		route.Dial = func(dialCtx context.Context, address string) (net.Conn, error) {
			if err := check(dialCtx); err != nil {
				return nil, err
			}
			conn, err := gateway.DialPublicContext(dialCtx, address)
			if err != nil {
				return nil, err
			}
			if closed.Load() || ctx.Err() != nil {
				conn.Close()
				return nil, ErrStaleAccountNetwork
			}
			return conn, nil
		}
		if strict {
			for i := 0; i < 2; i++ {
				if err := check(ctx); err != nil {
					closeFn()
					return nil, nil, "", err
				}
			}
		}
	}
	owned = true
	return &client, closeFn, v.Geo.IP, nil
}

// CredentialValidationClient reuses the committed account route and all its
// country, identity, generation and request-lifetime fences, even while an
// account is disabled and its credentials are being repaired by an admin.
func (c Container) CredentialValidationClient(ctx context.Context, account string) (*http.Client, func(), error) {
	if err := c.requireAccountNetworkReady(ctx, account); err != nil {
		return nil, nil, err
	}
	cfg, err := (Repository{DB: c.DB}).AccountForCleanup(ctx, account)
	if err != nil {
		return nil, nil, err
	}
	runtime, err := c.buildProviderNetworkRuntime(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return runtime.Client, runtime.CloseIdleConnections, nil
}
