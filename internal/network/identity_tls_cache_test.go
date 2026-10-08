package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdentityTLSFreshSocketsAndVerification(t *testing.T) {
	type observation struct {
		Resumed bool
		Remote  string
	}
	seen := make(chan observation, 32)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- observation{r.TLS.DidResume, r.RemoteAddr}
		io.WriteString(w, `{"Answer":[{"Type":1,"Data":"203.0.113.8","TTL":60}]}`)
	}))
	defer server.Close()
	var connects atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != server.Listener.Addr().String() || r.Header.Get("Proxy-Authorization") == "" {
			http.Error(w, "unexpected", 400)
			return
		}
		up, err := net.Dial("tcp4", r.Host)
		if err != nil {
			http.Error(w, "dial", 502)
			return
		}
		defer up.Close()
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		connects.Add(1)
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		up.SetDeadline(time.Now().Add(5 * time.Second))
		buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buf.Flush()
		done := make(chan struct{})
		go func() { io.Copy(up, buf); up.Close(); close(done) }()
		io.Copy(conn, up)
		conn.Close()
		<-done
	}))
	defer proxy.Close()
	host, portText, _ := net.SplitHostPort(proxy.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	p := Proxy{ID: t.Name(), Type: ProxyHTTP, Host: host, Port: port, Status: StatusHealthy}
	creds := ProxyCredentials{Username: "account-session", Password: "test"}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	makeGateway := func(owner string, cr ProxyCredentials, on bool) *Gateway {
		g, err := NewIdentityProxyGateway(owner, p, cr, on)
		if err != nil {
			t.Fatal(err)
		}
		if g.Transport.TLSClientConfig == nil {
			g.Transport.TLSClientConfig = &tls.Config{}
		}
		g.Transport.TLSClientConfig.RootCAs = roots
		return g
	}
	remote := map[string]bool{}
	probe := func(owner string, cr ProxyCredentials, on, wantResume bool) {
		g := makeGateway(owner, cr, on)
		defer g.CloseIdleConnections()
		resp, err := g.Client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		o := <-seen
		if o.Resumed != wantResume || remote[o.Remote] {
			t.Fatalf("resume=%t wanted=%t fresh=%t", o.Resumed, wantResume, !remote[o.Remote])
		}
		remote[o.Remote] = true
	}
	probe("a", creds, true, false)
	probe("a", creds, true, true)
	probe("b", creds, true, false)
	changed := creds
	changed.Password = "changed"
	probe("a", changed, true, false)
	changed = creds
	changed.Username = "new-sticky-session"
	probe("a", changed, true, false)
	probe("a", creds, false, false)
	probe("a", creds, false, false)
	// Cached verified chains must not bypass changed roots or hostname checks.
	bad := makeGateway("a", creds, true)
	bad.Transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	if resp, err := bad.Client.Get(server.URL); err == nil {
		resp.Body.Close()
		t.Fatal("untrusted certificate accepted")
	}
	bad.CloseIdleConnections()
	bad = makeGateway("a", creds, true)
	bad.Transport.TLSClientConfig.ServerName = "wrong.invalid"
	if resp, err := bad.Client.Get(server.URL); err == nil {
		resp.Body.Close()
		t.Fatal("wrong hostname accepted")
	}
	bad.CloseIdleConnections()
	// DNS uses the same scoped ticket cache, but a fresh resolver, CONNECT and
	// DNS response on each independent identity gateway.
	var previous *proxyAResolver
	for i := 0; i < 2; i++ {
		g := makeGateway("dns", creds, true)
		if g.resolver == previous || len(g.resolver.cache) != 0 {
			t.Fatal("DNS resolver/results shared")
		}
		previous = g.resolver
		g.resolver.tlsConfig.RootCAs = roots
		g.resolver.tlsConfig.ServerName = server.Certificate().DNSNames[0]
		dial := g.resolver.dial
		g.resolver.dial = func(ctx context.Context, target string) (net.Conn, error) {
			if target != "1.1.1.1:443" {
				return nil, fmt.Errorf("unexpected DNS target")
			}
			return dial(ctx, server.Listener.Addr().String())
		}
		ip, err := g.resolver.Resolve(context.Background(), "fresh.example")
		if err != nil || ip != "203.0.113.8" {
			t.Fatalf("DNS %s %v", ip, err)
		}
		o := <-seen
		if o.Resumed != (i == 1) || remote[o.Remote] {
			t.Fatalf("DNS resumed=%t fresh=%t", o.Resumed, !remote[o.Remote])
		}
		remote[o.Remote] = true
		g.CloseIdleConnections()
	}
	if connects.Load() != 11 {
		t.Fatalf("CONNECT count=%d", connects.Load())
	}
}

func TestIdentityTLSCacheIsolationBoundsExpiryRace(t *testing.T) {
	s := identityTLSCaches{entries: make(map[[32]byte]*identityTLSCache)}
	p := Proxy{ID: "p", Type: ProxyHTTP, Host: "127.0.0.1", Port: 8080}
	creds := ProxyCredentials{Username: "sticky", Password: "secret"}
	c := s.forRoute("a", p, creds)
	variants := []struct {
		owner string
		p     Proxy
		creds ProxyCredentials
	}{{"b", p, creds}}
	for _, field := range []string{"id", "type", "host", "port"} {
		v := p
		switch field {
		case "id":
			v.ID = "p2"
		case "type":
			v.Type = ProxyHTTPS
		case "host":
			v.Host = "127.0.0.2"
		case "port":
			v.Port++
		}
		variants = append(variants, struct {
			owner string
			p     Proxy
			creds ProxyCredentials
		}{"a", v, creds})
	}
	for _, v := range variants {
		if s.forRoute(v.owner, v.p, v.creds) == c {
			t.Fatal("route TLS state shared")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache := s.forRoute(strconv.Itoa(i), p, creds)
			cache.Put("server", &tls.ClientSessionState{})
			cache.Get("server")
			cache.Put("server", nil)
		}(i)
	}
	wg.Wait()
	if len(s.entries) != 128 {
		t.Fatalf("scope count=%d", len(s.entries))
	}
	c = s.forRoute("bounded", p, creds)
	expiry := c.expires
	for i := 0; i < 9; i++ {
		c.Put(strconv.Itoa(i), &tls.ClientSessionState{})
	}
	if _, ok := c.Get("0"); ok {
		t.Fatal("server-entry LRU exceeded eight")
	}
	if _, ok := c.Get("8"); !ok {
		t.Fatal("newest ticket lost")
	}
	if s.forRoute("bounded", p, creds) != c || !c.expires.Equal(expiry) {
		t.Fatal("expiry slides on use")
	}
	c.expires = time.Now().Add(-time.Second)
	if _, ok := c.Get("8"); ok {
		t.Fatal("expired retained reference used")
	}
	c.Put("expired", &tls.ClientSessionState{})
	if _, ok := c.cache.Get("expired"); ok {
		t.Fatal("expired cache accepted ticket")
	}
	if s.forRoute("bounded", p, creds) == c {
		t.Fatal("expired scope reused")
	}
}
