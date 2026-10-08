package network

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	xproxy "golang.org/x/net/proxy"
)

var (
	ErrUnsupportedProxyType = errors.New("unsupported proxy type")
	ErrProxyConfigInvalid   = errors.New("invalid proxy configuration")
)

type ProxyCredentials struct{ Username, Password string }

type Gateway struct {
	AccountID   string
	Proxy       Proxy
	Credentials ProxyCredentials
	Transport   *http.Transport
	Client      *http.Client
	pooled      bool
}

func NewProxyGateway(accountID string, p Proxy, creds ProxyCredentials) (*Gateway, error) {
	if p.Status != StatusHealthy {
		return nil, ErrProxyConfigInvalid
	}
	return newProxyGateway(accountID, p, creds)
}

func NewProxyProbeGateway(accountID string, p Proxy, creds ProxyCredentials) (*Gateway, error) {
	return newProxyGateway(accountID, p, creds)
}
func NewAccountProxyGateway(accountID string, p Proxy, creds ProxyCredentials, purpose string) (*Gateway, error) {
	if p.Status != StatusHealthy {
		return nil, ErrProxyConfigInvalid
	}
	return newProxyGatewayForPurpose(accountID, p, creds, purpose)
}
func newSOCKSLiteralDialer(p Proxy, creds ProxyCredentials) (literalProxyDial, error) {
	return newSOCKSLiteralDialerMeasured(p, creds, trafficScope{})
}
func newSOCKSLiteralDialerMeasured(p Proxy, creds ProxyCredentials, scope trafficScope) (literalProxyDial, error) {
	var auth *xproxy.Auth
	if creds.Username != "" {
		auth = &xproxy.Auth{User: creds.Username, Password: creds.Password}
	}
	endpoint, err := IPv4Endpoint(context.Background(), p.Host, p.Port)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, target string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil {
			return nil, ErrIPv4Required
		}
		dialer, err := xproxy.SOCKS5("tcp4", endpoint, auth, &meteredForwardDialer{scope: scopeForContext(scope, ctx)})
		if err != nil {
			return nil, err
		}
		if contextual, ok := dialer.(xproxy.ContextDialer); ok {
			return contextual.DialContext(ctx, "tcp4", target)
		}
		type result struct {
			c   net.Conn
			err error
		}
		ch := make(chan result, 1)
		go func() {
			c, e := dialer.Dial("tcp4", target)
			ch <- result{c: c, err: e}
		}()
		select {
		case <-ctx.Done():
			go func() {
				r := <-ch
				if r.c != nil {
					_ = r.c.Close()
				}
			}()
			return nil, ctx.Err()
		case r := <-ch:
			return r.c, r.err
		}
	}, nil
}

func newProxyGateway(accountID string, p Proxy, creds ProxyCredentials) (*Gateway, error) {
	return newProxyGatewayForPurpose(accountID, p, creds, "admin_probe")
}
func newProxyGatewayForPurpose(accountID string, p Proxy, creds ProxyCredentials, purpose string) (*Gateway, error) {
	if accountID == "" || p.Host == "" || p.Port < 1 {
		return nil, ErrProxyConfigInvalid
	}
	var dialLiteral literalProxyDial
	var err error
	scope := proxyTrafficScope(accountID, p.ID, purpose)
	switch p.Type {
	case ProxyHTTP, ProxyHTTPS:
		dialLiteral, err = httpProxyLiteralDialerMeasured(p, creds, scope)
	case ProxySOCKS5:
		dialLiteral, err = newSOCKSLiteralDialerMeasured(p, creds, scope)
	default:
		return nil, ErrUnsupportedProxyType
	}
	if err != nil {
		return nil, err
	}
	resolver := newProxyAResolver(accountID, dialLiteral)
	resolver.scope = scope
	tr := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
	tr.DialContext = func(ctx context.Context, _ string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ip, err := resolver.Resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		return dialLiteral(ctx, net.JoinHostPort(ip, port))
	}
	client := &http.Client{
		Transport: trafficTransport{base: MetadataTransport{Base: PrivacyTransport{Base: tr}}, scope: scope},
		Timeout:   30 * time.Second,
	}
	return &Gateway{AccountID: accountID, Proxy: p, Credentials: creds, Transport: tr, Client: client}, nil
}

func (g *Gateway) Validate(accountID string) error {
	if g == nil || g.AccountID != accountID {
		return ErrAccountContextMismatch
	}
	return nil
}
func (g *Gateway) CloseIdleConnections() {
	if g != nil && g.Transport != nil && !g.pooled {
		g.Transport.CloseIdleConnections()
	}
}
func (g *Gateway) DirectDial(context.Context, string, string) (net.Conn, error) {
	return nil, ErrProxyRequired
}
