package network

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Measurements count bytes at the socket to the proxy, including proxy auth,
// CONNECT and TLS. They exclude TCP/IP headers/retransmissions and cannot be
// equated to provider-billed traffic. HTTP responses are not buffered.
type trafficScope struct{ Role, Owner, Proxy, Purpose string }
type trafficKey struct {
	Hour  time.Time
	Scope trafficScope
}
type trafficCount struct{ TX, RX, Connections, Requests, Errors int64 }
type trafficMeter struct {
	mu      sync.Mutex
	flushMu sync.Mutex
	process string
	counts  map[trafficKey]trafficCount
}

func newTrafficMeter() *trafficMeter {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic("traffic process identity unavailable")
	}
	return &trafficMeter{process: hex.EncodeToString(id[:]), counts: make(map[trafficKey]trafficCount)}
}

var proxyTraffic = newTrafficMeter()

func proxyTrafficScope(owner, proxyID, purpose string) trafficScope {
	role := "account"
	if strings.HasPrefix(owner, "residential:") {
		role = "residential"
	}
	if strings.HasPrefix(owner, "proxy-monitor:") {
		role = "base_proxy"
	}
	if purpose == "" {
		purpose = "admin_probe"
	}
	return trafficScope{Role: role, Owner: owner, Proxy: proxyID, Purpose: purpose}
}

func (m *trafficMeter) add(s trafficScope, d trafficCount) {
	if s.Role == "" {
		return
	}
	k := trafficKey{time.Now().UTC().Truncate(time.Hour), s}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.counts[k]; !ok && len(m.counts) >= 4096 {
		// Keep global accounting bounded even during a long database outage.
		// An explicit overflow row reports attribution loss, never silent loss.
		k = trafficKey{time.Unix(0, 0).UTC(), trafficScope{Role: "overflow", Purpose: "attribution_overflow"}}
	}
	c := m.counts[k]
	c.TX += d.TX
	c.RX += d.RX
	c.Connections += d.Connections
	c.Requests += d.Requests
	c.Errors += d.Errors
	m.counts[k] = c
}

type meteredProxyConn struct {
	net.Conn
	meter *trafficMeter
	scope trafficScope
}

func (c *meteredProxyConn) Read(b []byte) (int, error) {
	n, e := c.Conn.Read(b)
	c.meter.add(c.scope, trafficCount{RX: int64(n)})
	return n, e
}
func (c *meteredProxyConn) Write(b []byte) (int, error) {
	n, e := c.Conn.Write(b)
	c.meter.add(c.scope, trafficCount{TX: int64(n)})
	return n, e
}
func meterProxyConn(c net.Conn, scope trafficScope) net.Conn {
	if scope.Role == "" {
		return c
	}
	proxyTraffic.add(scope, trafficCount{Connections: 1})
	return &meteredProxyConn{Conn: c, meter: proxyTraffic, scope: scope}
}

type meteredForwardDialer struct{ scope trafficScope }

func (d *meteredForwardDialer) Dial(n, a string) (net.Conn, error) {
	return d.DialContext(context.Background(), n, a)
}
func (d *meteredForwardDialer) DialContext(ctx context.Context, n, a string) (net.Conn, error) {
	c, e := DialContextIPv4(ctx, n, a)
	if e != nil {
		return nil, e
	}
	return meterProxyConn(c, d.scope), nil
}

type trafficPurposeKey struct{}

func withTrafficPurpose(ctx context.Context, p string) context.Context {
	return context.WithValue(ctx, trafficPurposeKey{}, p)
}
func scopeForContext(scope trafficScope, ctx context.Context) trafficScope {
	if p, ok := ctx.Value(trafficPurposeKey{}).(string); ok {
		scope.Purpose = p
	}
	return scope
}

type trafficTransport struct {
	base  http.RoundTripper
	scope trafficScope
}

func (t trafficTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	d := trafficCount{Requests: 1}
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		d.Errors = 1
	}
	proxyTraffic.add(t.scope, d)
	return resp, err
}

func (m *trafficMeter) flush(ctx context.Context, db *sql.DB) error {
	m.flushMu.Lock()
	defer m.flushMu.Unlock()
	m.mu.Lock()
	rows := make(map[trafficKey]trafficCount, len(m.counts))
	for k, v := range m.counts {
		rows[k] = v
	}
	m.mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range rows {
		_, err = tx.ExecContext(ctx, `INSERT INTO proxy_traffic_hourly(hour,process_id,role,owner_id,proxy_id,purpose,tx_bytes,rx_bytes,connections,requests,errors)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(hour,process_id,role,owner_id,proxy_id,purpose) DO UPDATE SET
tx_bytes=GREATEST(proxy_traffic_hourly.tx_bytes,EXCLUDED.tx_bytes),rx_bytes=GREATEST(proxy_traffic_hourly.rx_bytes,EXCLUDED.rx_bytes),
connections=GREATEST(proxy_traffic_hourly.connections,EXCLUDED.connections),requests=GREATEST(proxy_traffic_hourly.requests,EXCLUDED.requests),
errors=GREATEST(proxy_traffic_hourly.errors,EXCLUDED.errors),updated_at=now()`,
			k.Hour, m.process, k.Scope.Role, k.Scope.Owner, k.Scope.Proxy, k.Scope.Purpose, v.TX, v.RX, v.Connections, v.Requests, v.Errors)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range rows {
		// add() timestamps at observation time; completed hours cannot receive
		// delayed socket counts. Retain overflow for process-lifetime monotonicity.
		if k.Scope.Role != "overflow" && time.Since(k.Hour.Add(time.Hour)) > 2*time.Minute && m.counts[k] == v {
			delete(m.counts, k)
		}
	}
	return nil
}

// StartProxyTrafficRecorder returns a bounded shutdown flush. A crash may lose
// at most the unflushed interval; this is measured telemetry, not a billing ledger.
func StartProxyTrafficRecorder(_ context.Context, db *sql.DB) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		flush := func() {
			c, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			if e := proxyTraffic.flush(c, db); e != nil {
				log.Printf("proxy traffic flush unavailable: %T", e)
			}
		}
		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case <-t.C:
				flush()
			}
		}
	}()
	return func() { cancel(); <-done }
}
