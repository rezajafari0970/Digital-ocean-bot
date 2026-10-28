package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

type countryTransportFunc func(*http.Request) (*http.Response, error)

func (f countryTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCountryCandidateNativeTunnelAndReconnectPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c, a, _ := countryFixture(t, db, `["US","TR"]`)
	s, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	var rotating, wrongCountry, observerOutage, rejectAuth, fixedMode, missingCountry atomic.Bool
	var connects, geoCalls, ipCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Fixture-Observer") == "ipwho.is" {
			geoCalls.Add(1)
			if observerOutage.Load() {
				http.Error(w, "observer outage", 503)
				return
			}
			country := "US"
			if wrongCountry.Load() {
				country = "TR"
			}
			if missingCountry.Load() {
				country = ""
			}
			json.NewEncoder(w).Encode(map[string]any{"ip": "198.18.64.2", "country": "Fixture country", "country_code": country, "timezone": map[string]string{"id": "America/New_York"}, "connection": map[string]any{"asn": 64500, "org": "fixture"}})
			return
		}
		ipCalls.Add(1)
		ip := "198.18.64.2"
		if rotating.Load() {
			ip = "198.18.65.2"
		}
		json.NewEncoder(w).Encode(map[string]string{"ip": ip})
	}))
	defer server.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rejectAuth.Load() {
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		if r.Method != "CONNECT" || r.Host != server.Listener.Addr().String() {
			http.Error(w, "unexpected tunnel", 400)
			return
		}
		auth := r.Clone(r.Context())
		auth.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
		user, _, ok := auth.BasicAuth()
		if !ok || (!fixedMode.Load() && !strings.Contains(user, "cr.us;sid.native-proof")) || (fixedMode.Load() && user != "base") {
			http.Error(w, "wrong driver grammar", 407)
			return
		}
		up, err := net.DialTimeout("tcp4", r.Host, time.Second)
		if err != nil {
			http.Error(w, "fixture unavailable", 502)
			return
		}
		defer up.Close()
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		connects.Add(1)
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		up.SetDeadline(time.Now().Add(10 * time.Second))
		buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buf.Flush()
		done := make(chan struct{})
		go func() { io.Copy(up, buf); up.Close(); close(done) }()
		io.Copy(conn, up)
		conn.Close()
		<-done
	}))
	defer proxy.Close()
	host, portString, _ := net.SplitHostPort(proxy.Listener.Addr().String())
	port, _ := strconv.Atoi(portString)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	target, _ := url.Parse(server.URL)
	factory := func(ctx context.Context, account string, p network.Proxy, creds network.ProxyCredentials) (*network.Gateway, error) {
		p.Host = host
		p.Port = port
		g, err := network.NewIdentityProxyGateway(account, p, creds, false)
		if err != nil {
			return nil, err
		}
		g.Transport.TLSClientConfig = &tls.Config{RootCAs: roots}
		base := g.Client.Transport
		g.Client.Transport = countryTransportFunc(func(r *http.Request) (*http.Response, error) {
			copy := r.Clone(r.Context())
			u := *r.URL
			u.Scheme = target.Scheme
			u.Host = target.Host
			copy.URL = &u
			copy.Host = target.Host
			copy.Header.Set("X-Fixture-Observer", r.URL.Hostname())
			return base.RoundTrip(copy)
		})
		return g, nil
	}
	p := s.Config.Pool[0]
	v := countryCandidate{ProxyID: p.ID, Session: "native-proof", Target: "US", Port: p.Port}
	proof, result := c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	if result.Health != proxyselection.Usable || proof.Geo.IP != "198.18.64.2" || proof.Geo.ASN != "AS64500 fixture" {
		t.Fatal("real tunnel proof failed", result, proof.Geo)
	}
	if connects.Load() < 3 || geoCalls.Load() != 1 || ipCalls.Load() < 2 {
		t.Fatal("fresh connections not verified", connects.Load(), geoCalls.Load(), ipCalls.Load())
	}
	rotating.Store(true)
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	if result.Health != proxyselection.Unusable {
		t.Fatal("rotating identity accepted", result)
	}
	rotating.Store(false)
	wrongCountry.Store(true)
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	if result.Health != proxyselection.Unusable {
		t.Fatal("wrong country accepted", result)
	}
	wrongCountry.Store(false)
	observerOutage.Store(true)
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	if result.Health != proxyselection.Unknown {
		t.Fatal("observer outage authorized lower country", result)
	}
	observerOutage.Store(false)
	missingCountry.Store(true)
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	if result.Health != proxyselection.Unknown {
		t.Fatal("missing country authorized descent", result)
	}
	missingCountry.Store(false)
	// A generic rotating endpoint cannot opt into explicit country guarantees.
	before := connects.Load()
	generic := p
	generic.Adapter = "generic"
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, generic, v, factory)
	if result.Health != proxyselection.Unusable || connects.Load() != before {
		t.Fatal("generic driver inferred country stickiness", result)
	}
	fixedMode.Store(true)
	fixed := p
	fixed.Adapter = "fixed"
	fixedV := v
	fixedV.Session = ""
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, fixed, fixedV, factory)
	if result.Health != proxyselection.Usable {
		t.Fatal("independent fixed-egress driver failed", result)
	}
	wrongCountry.Store(true)
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, fixed, fixedV, factory)
	if result.Health != proxyselection.Unusable || !result.ExhaustProvider {
		t.Fatal("fixed country mismatch retried as a rotating pool", result)
	}
	wrongCountry.Store(false)
	fixedMode.Store(false)
	rejectAuth.Store(true)
	_, result = c.verifyCountryCandidateWithGateway(ctx, s, p, v, factory)
	if result.Health != proxyselection.Unusable || !result.ExhaustProvider {
		t.Fatal("auth rejection not isolated to provider", result)
	}
}
