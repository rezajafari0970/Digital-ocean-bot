package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
}

func NewProxyGateway(accountID string, p Proxy, creds ProxyCredentials) (*Gateway, error) {
	if accountID == "" || p.Host == "" || p.Port < 1 || p.Status != StatusHealthy {
		return nil, ErrProxyConfigInvalid
	}
	tr := &http.Transport{ForceAttemptHTTP2: true, MaxIdleConns: 20, IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 10 * time.Second}
	switch p.Type {
	case ProxyHTTP, ProxyHTTPS:
		scheme := "http"
		if p.Type == ProxyHTTPS {
			scheme = "https"
		}
		u := &url.URL{Scheme: scheme, Host: net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))}
		if creds.Username != "" {
			u.User = url.UserPassword(creds.Username, creds.Password)
		}
		tr.Proxy = http.ProxyURL(u)
		tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	case ProxySOCKS5:
		var auth *xproxy.Auth
		if creds.Username != "" {
			auth = &xproxy.Auth{User: creds.Username, Password: creds.Password}
		}
		dialer, err := xproxy.SOCKS5("tcp", net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port)), auth, xproxy.Direct)
		if err != nil {
			return nil, err
		}
		tr.Proxy = nil
		tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			type result struct {
				c   net.Conn
				err error
			}
			ch := make(chan result, 1)
			go func() { c, err := dialer.Dial(network, address); ch <- result{c, err} }()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case r := <-ch:
				return r.c, r.err
			}
		}
	default:
		return nil, ErrUnsupportedProxyType
	}
	client := &http.Client{Transport: MetadataTransport{Base: PrivacyTransport{Base: tr}}, Timeout: 30 * time.Second}
	return &Gateway{AccountID: accountID, Proxy: p, Credentials: creds, Transport: tr, Client: client}, nil
}

func (g *Gateway) Validate(accountID string) error {
	if g == nil || g.AccountID != accountID {
		return ErrAccountContextMismatch
	}
	return nil
}
func (g *Gateway) CloseIdleConnections() {
	if g != nil && g.Transport != nil {
		g.Transport.CloseIdleConnections()
	}
}
func (g *Gateway) DirectDial(context.Context, string, string) (net.Conn, error) {
	return nil, ErrProxyRequired
}
