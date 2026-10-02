package network

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestProxyResolverRejectsIPv6WithoutDial(t *testing.T) {
	called := false
	r := newProxyAResolver("acct", func(context.Context, string) (net.Conn, error) {
		called = true
		return nil, errors.New("unexpected dial")
	})
	if _, err := r.Resolve(context.Background(), "2001:db8::1"); !errors.Is(err, ErrIPv6Prohibited) {
		t.Fatalf("err=%v", err)
	}
	if called {
		t.Fatal("IPv6 literal must fail before proxy/DNS dial")
	}
}

func TestHTTPProxyLiteralDialerRejectsIPv6Target(t *testing.T) {
	d, err := httpProxyLiteralDialer(Proxy{Type: ProxyHTTP, Host: "127.0.0.1", Port: 9}, ProxyCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d(context.Background(), "[2001:db8::1]:443"); !errors.Is(err, ErrIPv4Required) {
		t.Fatalf("err=%v", err)
	}
}

func TestProxyResolverAcceptsIPv4LiteralWithoutDial(t *testing.T) {
	called := false
	r := newProxyAResolver("acct", func(context.Context, string) (net.Conn, error) {
		called = true
		return nil, errors.New("unexpected dial")
	})
	got, err := r.Resolve(context.Background(), "203.0.113.8")
	if err != nil || got != "203.0.113.8" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if called {
		t.Fatal("literal IPv4 must not resolve")
	}
}

func TestHTTPProxyConnectUsesIPv4LiteralTarget(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	targetCh := make(chan string, 1)
	authCh := make(chan string, 1)
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		req, e := http.ReadRequest(bufio.NewReader(c))
		if e != nil {
			return
		}
		targetCh <- req.RequestURI
		authCh <- req.Header.Get("Proxy-Authorization")
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		time.Sleep(50 * time.Millisecond)
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	dial, err := httpProxyLiteralDialer(Proxy{Type: ProxyHTTP, Host: "127.0.0.1", Port: port}, ProxyCredentials{Username: "acct__sid.unique", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := dial(context.Background(), "203.0.113.9:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if got := <-targetCh; got != "203.0.113.9:443" {
		t.Fatalf("CONNECT target=%q", got)
	}
	if got := <-authCh; got == "" {
		t.Fatal("account-scoped proxy credentials missing from CONNECT")
	}
}

func TestProxyResolverFailsClosedWhenProxyPathFails(t *testing.T) {
	calls := 0
	r := newProxyAResolver("acct", func(_ context.Context, target string) (net.Conn, error) {
		calls++
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			t.Fatalf("target=%q err=%v", target, err)
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil {
			t.Fatalf("non-IPv4 DoH target %q", target)
		}
		return nil, errors.New("proxy unavailable")
	})
	if _, err := r.Resolve(context.Background(), "api.example.test"); err == nil {
		t.Fatal("proxy DNS must fail closed")
	}
	if calls != 2 {
		t.Fatalf("proxied DoH attempts=%d want=2", calls)
	}
}
