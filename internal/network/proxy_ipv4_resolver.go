package network

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrProxyDNS       = errors.New("proxy DNS A lookup failed")
	ErrProxyConnect   = errors.New("proxy CONNECT failed")
	ErrProxyAuth      = errors.New("proxy authentication failed")
	ErrIPv6Prohibited = errors.New("IPv6 prohibited")
)

type literalProxyDial func(context.Context, string) (net.Conn, error)
type cachedA struct {
	ip      string
	expires time.Time
}
type proxyAResolver struct {
	accountID string
	dial      literalProxyDial
	mu        sync.Mutex
	cache     map[string]cachedA
	scope     trafficScope
	pending   map[string]*dnsFlight
}

type dnsFlight struct {
	done chan struct{}
	ip   string
	err  error
}

func newProxyAResolver(accountID string, dial literalProxyDial) *proxyAResolver {
	return &proxyAResolver{accountID: accountID, dial: dial, cache: make(map[string]cachedA), pending: make(map[string]*dnsFlight)}
}
func (r *proxyAResolver) Resolve(ctx context.Context, host string) (string, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), nil
		}
		return "", ErrIPv6Prohibited
	}
	now := time.Now()
	r.mu.Lock()
	if x, ok := r.cache[host]; ok && now.Before(x.expires) {
		r.mu.Unlock()
		return x.ip, nil
	}
	if flight := r.pending[host]; flight != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-flight.done:
			return flight.ip, flight.err
		}
	}
	flight := &dnsFlight{done: make(chan struct{})}
	if len(r.pending) >= 256 {
		r.mu.Unlock()
		return "", ErrProxyDNS
	}
	r.pending[host] = flight
	r.mu.Unlock()
	ip, err := r.resolveFresh(ctx, host)
	r.mu.Lock()
	flight.ip = ip
	flight.err = err
	delete(r.pending, host)
	close(flight.done)
	r.mu.Unlock()
	return ip, err
}
func (r *proxyAResolver) resolveFresh(ctx context.Context, host string) (string, error) {
	endpoints := []struct{ name, ip, endpoint string }{
		{"cloudflare-dns.com", "1.1.1.1", "https://cloudflare-dns.com/dns-query"},
		{"dns.google", "8.8.8.8", "https://dns.google/resolve"},
	}
	var last error
	for _, ep := range endpoints {
		tr := &http.Transport{
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
				return r.dial(withTrafficPurpose(c, "dns"), net.JoinHostPort(ep.ip, "443"))
			},
		}
		q := ep.endpoint + "?name=" + url.QueryEscape(host) + "&type=A"
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, q, nil)
		req.Header.Set("Accept", "application/dns-json")
		dnsScope := r.scope
		dnsScope.Purpose = "dns"
		resp, err := (&http.Client{Transport: trafficTransport{base: tr, scope: dnsScope}, Timeout: 6 * time.Second}).Do(req)
		if err != nil {
			last = err
			tr.CloseIdleConnections()
			continue
		}
		var raw struct {
			Answer []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
				TTL  uint32 `json:"TTL"`
			} `json:"Answer"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&raw)
		resp.Body.Close()
		tr.CloseIdleConnections()
		if err != nil || resp.StatusCode/100 != 2 {
			last = ErrProxyDNS
			continue
		}
		ttl := uint32(60)
		for _, ans := range raw.Answer {
			if ans.TTL < ttl {
				ttl = ans.TTL
			}
		}
		for _, ans := range raw.Answer {
			if ans.Type != 1 {
				continue
			}
			ip := net.ParseIP(strings.TrimSpace(ans.Data))
			if ip == nil || ip.To4() == nil {
				continue
			}
			v := ip.To4().String()
			r.mu.Lock()
			for key, value := range r.cache {
				if !time.Now().Before(value.expires) {
					delete(r.cache, key)
				}
			}
			if len(r.cache) >= 256 {
				for key := range r.cache {
					delete(r.cache, key)
					break
				}
			}
			if ttl > 0 {
				r.cache[host] = cachedA{ip: v, expires: time.Now().Add(time.Duration(ttl) * time.Second)}
			}
			r.mu.Unlock()
			return v, nil
		}
		last = ErrProxyDNS
	}
	if last == nil {
		last = ErrProxyDNS
	}
	return "", last
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func httpProxyLiteralDialer(p Proxy, creds ProxyCredentials) (literalProxyDial, error) {
	return httpProxyLiteralDialerMeasured(p, creds, trafficScope{})
}
func httpProxyLiteralDialerMeasured(p Proxy, creds ProxyCredentials, scope trafficScope) (literalProxyDial, error) {
	endpoint, err := IPv4Endpoint(context.Background(), p.Host, p.Port)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, target string) (net.Conn, error) {
		h, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		ip := net.ParseIP(strings.Trim(h, "[]"))
		if ip == nil || ip.To4() == nil {
			return nil, ErrIPv4Required
		}
		base := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		conn, err := base.DialContext(ctx, "tcp4", endpoint)
		if err != nil {
			return nil, err
		}
		conn = meterProxyConn(conn, scopeForContext(scope, ctx))
		fail := func(e error) (net.Conn, error) { _ = conn.Close(); return nil, e }
		if p.Type == ProxyHTTPS {
			tc := tls.Client(conn, &tls.Config{ServerName: strings.Trim(p.Host, "[]"), MinVersion: tls.VersionTLS12})
			if err := tc.HandshakeContext(ctx); err != nil {
				return fail(err)
			}
			conn = tc
		}
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(deadline)
		}
		auth := ""
		if creds.Username != "" {
			auth = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(creds.Username+":"+creds.Password)) + "\r\n"
		}
		if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n%sProxy-Connection: Keep-Alive\r\n\r\n", target, target, auth); err != nil {
			return fail(err)
		}
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
		if err != nil {
			return fail(err)
		}
		if resp.StatusCode/100 != 2 {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			if resp.StatusCode == http.StatusProxyAuthRequired {
				return fail(fmt.Errorf("%w: %s", ErrProxyAuth, resp.Status))
			}
			return fail(fmt.Errorf("%w: %s", ErrProxyConnect, resp.Status))
		}
		_ = conn.SetDeadline(time.Time{})
		return &bufferedConn{Conn: conn, r: br}, nil
	}, nil
}
