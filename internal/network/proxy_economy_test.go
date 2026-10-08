package network

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestEconomyPoolIsolationRace(t *testing.T) {
	pool := NewProviderTransportPool(3)
	defer pool.Close()
	p := Proxy{ID: "p", Type: ProxyHTTP, Host: "127.0.0.1", Port: 9, Status: StatusHealthy}
	creds := ProxyCredentials{Username: "u", Password: "p"}
	a, err := pool.Open("a", p, creds, "1")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := pool.Open("a", p, creds, "1")
			if err != nil {
				t.Error(err)
				return
			}
			if a.Transport != b.Transport || a.Client == b.Client {
				t.Error("shared mutable client or missing raw reuse")
			}
			b.Client.Transport = nil
			b.CloseIdleConnections()
		}()
	}
	wg.Wait()
	b, err := pool.Open("b", p, creds, "1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Transport == b.Transport {
		t.Fatal("cross-account reuse")
	}
	newer, err := pool.Open("a", p, creds, "2")
	if err != nil || newer.Transport == a.Transport {
		t.Fatal("stale generation reused", err)
	}
	creds.Password = "changed"
	changed, err := pool.Open("a", p, creds, "2")
	if err != nil || changed.Transport == newer.Transport {
		t.Fatal("credential reuse", err)
	}
	p.Status = StatusDown
	if _, err := pool.Open("a", p, creds, "2"); err == nil {
		t.Fatal("down proxy reused")
	}
	if a.Client.Transport == nil {
		t.Fatal("client wrapper mutation leaked")
	}
	if a.Transport.TLSClientConfig.ClientSessionCache != a.resolver.tlsConfig.ClientSessionCache {
		t.Fatal("destination/DNS ticket cache mismatch")
	}
	for _, g := range []*Gateway{b, newer, changed} {
		if a.Transport.TLSClientConfig.ClientSessionCache == g.Transport.TLSClientConfig.ClientSessionCache {
			t.Fatal("cross-scope TLS tickets")
		}
	}
	plain, err := NewAccountProxyGateway("a", Proxy{ID: "p", Type: ProxyHTTP, Host: "127.0.0.1", Port: 9, Status: StatusHealthy}, creds, "provider_api")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Transport.TLSClientConfig != nil || plain.resolver.tlsConfig != nil {
		t.Fatal("unpooled behavior changed")
	}

}
func TestEconomyBaseIntervalsFault(t *testing.T) {
	now := time.Now()
	cases := []struct {
		s        ProxyStatus
		fail, ok int
		diag     string
		age      time.Duration
		wait     bool
	}{
		{StatusHealthy, 0, 2, "", 299 * time.Second, true}, {StatusHealthy, 0, 2, "", 300 * time.Second, false},
		{StatusDegraded, 0, 1, "", time.Second, false}, {StatusDown, 3, 0, "PROXY_TIMEOUT", time.Second, false},
		{StatusDown, 2, 0, "PROXY_AUTH_FAILED", 299 * time.Second, true}, {StatusDown, 2, 0, "PROXY_AUTH_FAILED", 300 * time.Second, false},
		{StatusHealthy, 0, 2, "", -time.Second, false},
	}
	for _, c := range cases {
		if got := baseProbeMayWait(c.s, c.fail, c.ok, c.diag, now.Add(-c.age), now); got != c.wait {
			t.Fatalf("case=%+v got=%t", c, got)
		}
	}
}
func trafficTotals(scope trafficScope) trafficCount {
	proxyTraffic.mu.Lock()
	defer proxyTraffic.mu.Unlock()
	var out trafficCount
	for k, v := range proxyTraffic.counts {
		if k.Scope == scope {
			out.TX += v.TX
			out.RX += v.RX
			out.Connections += v.Connections
		}
	}
	return out
}
func TestEconomyHTTPConnectSocketAccounting(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan int64, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		br := bufio.NewReader(c)
		var n int64
		for {
			line, err := br.ReadString('\n')
			n += int64(len(line))
			if err != nil || line == "\r\n" {
				break
			}
		}
		io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\nabc")
		buf := make([]byte, 4)
		m, _ := io.ReadFull(br, buf)
		got <- n + int64(m)
	}()
	scope := proxyTrafficScope("economy-connect", "p", "provider_api")
	before := trafficTotals(scope)
	d, err := httpProxyLiteralDialerMeasured(Proxy{Type: ProxyHTTP, Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}, ProxyCredentials{Username: "u", Password: "secret"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := d(ctx, "203.0.113.8:443")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	if _, err = io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	c.Close()
	wire := <-got
	after := trafficTotals(scope)
	if after.TX-before.TX != wire || after.RX-before.RX != int64(len("HTTP/1.1 200 Connection Established\r\n\r\nabc")) || after.Connections-before.Connections != 1 {
		t.Fatalf("before=%+v after=%+v wire=%d", before, after, wire)
	}
}
func TestEconomyDNSCoalescingFault(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	r := newProxyAResolver("a", func(ctx context.Context, _ string) (net.Conn, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, errors.New("offline")
	})
	done := make(chan struct{})
	go func() { r.Resolve(context.Background(), "test.invalid"); close(done) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Resolve(ctx, "test.invalid"); !errors.Is(err, context.Canceled) {
		t.Fatal("waiter cancellation", err)
	}
	close(release)
	<-done
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) != 0 || len(r.cache) != 0 {
		t.Fatal("failed DNS cached or flight leaked")
	}
}
func TestEconomyMeterRetryAndBoundPostgres(t *testing.T) {
	db := healthDiagnosticsDB(t)
	ctx := context.Background()
	m := newTrafficMeter()
	s := proxyTrafficScope("a", "p", "identity")
	m.add(s, trafficCount{TX: 11, RX: 22, Requests: 1, Connections: 1})
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.flush(cancelCtx, db); err == nil {
		t.Fatal("canceled flush succeeded")
	}
	for i := 0; i < 2; i++ {
		if err := m.flush(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	m.add(s, trafficCount{TX: 3})
	if err := m.flush(ctx, db); err != nil {
		t.Fatal(err)
	}
	var tx, rx int64
	if err := db.QueryRow(`SELECT sum(tx_bytes),sum(rx_bytes) FROM proxy_traffic_hourly WHERE process_id=$1`, m.process).Scan(&tx, &rx); err != nil || tx != 14 || rx != 22 {
		t.Fatalf("tx=%d rx=%d err=%v", tx, rx, err)
	}
	for i := 0; i < 5000; i++ {
		m.add(trafficScope{Role: "account", Owner: time.Unix(int64(i), 0).String(), Purpose: "identity"}, trafficCount{TX: 1})
	}
	if len(m.counts) > 4097 {
		t.Fatal("meter unbounded")
	}
	if err := m.flush(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT sum(tx_bytes) FROM proxy_traffic_hourly WHERE process_id=$1`, m.process).Scan(&tx); err != nil || tx != 5014 {
		t.Fatalf("overflow lost counts tx=%d err=%v", tx, err)
	}
}

var _ http.RoundTripper = trafficTransport{}

func TestEconomyRecorderOutlivesBootstrapAndFlushesPostgres(t *testing.T) {
	db := healthDiagnosticsDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stop := StartProxyTrafficRecorder(ctx, db)
	// A startup context may already be canceled while the process keeps serving.
	time.Sleep(25 * time.Millisecond)
	scope := proxyTrafficScope("bootstrap-lifetime-fixture", "p", "identity")
	proxyTraffic.add(scope, trafficCount{TX: 123, RX: 456})
	stop()
	var tx, rx int64
	if err := db.QueryRow(`SELECT sum(tx_bytes),sum(rx_bytes) FROM proxy_traffic_hourly WHERE owner_id=$1`, scope.Owner).Scan(&tx, &rx); err != nil || tx < 123 || rx < 456 {
		t.Fatalf("missing final flush tx=%d rx=%d err=%v", tx, rx, err)
	}
}
