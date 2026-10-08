package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// Real RFC1929 rejection on a local SOCKS socket, with no upstream dial.
func TestActiveProbeSOCKSAuthenticationDiagnostic(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		h := make([]byte, 2)
		if _, err = io.ReadFull(c, h); err != nil {
			done <- err
			return
		}
		if _, err = io.CopyN(io.Discard, c, int64(h[1])); err != nil {
			done <- err
			return
		}
		if _, err = c.Write([]byte{5, 2}); err != nil {
			done <- err
			return
		}
		if _, err = io.ReadFull(c, h); err != nil {
			done <- err
			return
		}
		if _, err = io.CopyN(io.Discard, c, int64(h[1])); err != nil {
			done <- err
			return
		}
		if _, err = io.ReadFull(c, h[:1]); err != nil {
			done <- err
			return
		}
		if _, err = io.CopyN(io.Discard, c, int64(h[0])); err != nil {
			done <- err
			return
		}
		_, err = c.Write([]byte{1, 1})
		done <- err
	}()
	g, err := NewProxyGateway("diagnostic-test", Proxy{ID: "test", Type: ProxySOCKS5, Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Status: StatusHealthy}, ProxyCredentials{Username: "USER_SENTINEL", Password: "PASSWORD_SENTINEL"})
	if err != nil {
		t.Fatal(err)
	}
	defer g.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	beforeMeter := trafficTotals(proxyTrafficScope("diagnostic-test", "test", "admin_probe"))
	result := CheckProxy(ctx, g, "https://198.51.100.10/health?token=URL_SECRET_SENTINEL", "", "")
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	afterMeter := trafficTotals(proxyTrafficScope("diagnostic-test", "test", "admin_probe"))
	if afterMeter.Connections-beforeMeter.Connections != 1 || afterMeter.TX-beforeMeter.TX < 20 || afterMeter.RX-beforeMeter.RX != 4 {
		t.Fatalf("SOCKS wire counters before=%+v after=%+v", beforeMeter, afterMeter)
	}
	if result.Status != StatusDown || result.Error != "PROXY_AUTH_FAILED" {
		t.Fatalf("missing safe authentication diagnosis: status=%s error=%q", result.Status, result.Error)
	}
}

func TestActiveHealthDiagnosticClosedVocabulary(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("endpoint?token=SECRET: %w", &url.Error{Op: "GET", URL: "https://USER:PASSWORD@example.invalid/?token=SECRET", Err: errors.New("username/password authentication failed")}), "PROXY_AUTH_FAILED"},
		{fmt.Errorf("SECRET: %w", ErrProxyAuth), "PROXY_AUTH_FAILED"},
		{errors.New("no acceptable authentication methods"), "PROXY_AUTH_METHOD_UNSUPPORTED"},
		{errors.New("url=SECRET username/password authentication failed"), "PROXY_TRANSPORT_FAILED"},
		{fmt.Errorf("SECRET: %w", context.DeadlineExceeded), "PROXY_TIMEOUT"},
		{context.Canceled, "PROXY_CHECK_CANCELED"},
		{ErrServerIPLeak, "PROXY_SERVER_IP_LEAK"},
		{ErrForbiddenOutboundHeader, "PROXY_HEADER_LEAK"},
		{ErrUnexpectedExitIP, "PROXY_EXIT_IP_MISMATCH"},
		{ErrHealthPayload, "PROXY_ENDPOINT_PAYLOAD_INVALID"},
		{ErrHealthHTTP, "PROXY_ENDPOINT_HTTP_FAILED"},
		{ErrProxyConfigInvalid, "PROXY_CONFIG_INVALID"},
	}
	for _, x := range cases {
		if got := activeProbeDiagnostic(x.err); got != x.want {
			t.Fatalf("got=%q want=%q", got, x.want)
		}
	}
	if got := NormalizeHealthDiagnostic("PROXY_AUTH_FAILED SECRET"); got != "PROXY_HEALTH_FAILED" {
		t.Fatal(got)
	}
	if got := NormalizeHealthDiagnostic("secret unavailable"); got != "PROXY_SECRET_UNAVAILABLE" {
		t.Fatal(got)
	}
}
func TestActiveHealthHTTPProxyAuthAndEndpointFailures(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusProxyAuthRequired) }))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	host, portText, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portText)
	g, err := NewProxyGateway("test", Proxy{ID: "test", Type: ProxyHTTP, Host: host, Port: port, Status: StatusHealthy}, ProxyCredentials{Username: "USER", Password: "SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	defer g.CloseIdleConnections()
	if got := CheckProxy(context.Background(), g, "https://198.51.100.10/", "", ""); got.Error != "PROXY_AUTH_FAILED" || got.Status != StatusDown {
		t.Fatalf("%+v", got)
	}
	for _, x := range []struct {
		status     int
		body, want string
	}{{503, "SECRET", "PROXY_ENDPOINT_HTTP_FAILED"}, {200, "not json SECRET", "PROXY_ENDPOINT_PAYLOAD_INVALID"}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(x.status); _, _ = w.Write([]byte(x.body)) }))
		got := CheckProxy(context.Background(), &Gateway{Client: srv.Client()}, srv.URL, "", "")
		srv.Close()
		if got.Error != x.want || got.Status != StatusDown || got.Latency <= 0 {
			t.Fatalf("%+v", got)
		}
	}
}
func TestHealthDiagnosticRecoveryKeepsHysteresis(t *testing.T) {
	policy := HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: time.Second}
	now := time.Now().UTC()
	state := HealthState{Status: StatusHealthy, LastSuccessAt: now.Add(-time.Minute)}
	state = state.Apply(HealthResult{Status: StatusDown, CheckedAt: now, Latency: 7 * time.Millisecond, Error: "PROXY_AUTH_FAILED"}, policy)
	if state.Status != StatusDegraded || state.LastError != "PROXY_AUTH_FAILED" || state.LastLatency != 7*time.Millisecond {
		t.Fatalf("%+v", state)
	}
	state = state.Apply(HealthResult{Status: StatusDown, CheckedAt: now.Add(time.Second), Error: "URL_SECRET"}, policy)
	if state.Status != StatusDown || state.LastError != "PROXY_HEALTH_FAILED" || state.ConsecutiveFailures != 2 {
		t.Fatalf("%+v", state)
	}
	state = state.Apply(HealthResult{Status: StatusHealthy, CheckedAt: now.Add(2 * time.Second), Latency: time.Millisecond}, policy)
	if state.Status != StatusDegraded || state.LastError != "" || state.ConsecutiveSuccesses != 1 {
		t.Fatalf("%+v", state)
	}
	state = state.Apply(HealthResult{Status: StatusHealthy, CheckedAt: now.Add(3 * time.Second), Latency: time.Millisecond}, policy)
	if state.Status != StatusHealthy || state.ConsecutiveFailures != 0 {
		t.Fatalf("%+v", state)
	}
	state = state.Apply(HealthResult{Status: StatusHealthy, CheckedAt: now.Add(4 * time.Second), Latency: 2 * time.Second}, policy)
	if state.Status != StatusDegraded || state.LastError != "PROXY_LATENCY_EXCEEDED" {
		t.Fatalf("%+v", state)
	}
}
