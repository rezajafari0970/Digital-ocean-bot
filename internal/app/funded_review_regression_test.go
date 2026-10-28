package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
	"golang.org/x/net/http2/hpack"
)

func TestFundedReviewDirectRouteFencePostgres(t *testing.T) {
	for _, mode := range []string{"captured-before-build", "already-built", "direct-proxy-direct"} {
		t.Run(mode, func(t *testing.T) {
			db := installerBootstrapDB(t)
			ctx := context.Background()
			c, a, ps := countryFixture(t, db, "")
			execBootstrap(t, db, "UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1", a)
			cfg, err := c.Accounts.Account(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(204)
			}))
			defer server.Close()
			var rt providerNetworkRuntime
			if mode != "captured-before-build" {
				rt, err = c.buildProviderNetworkRuntime(ctx, cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer rt.CloseIdleConnections()
			}
			execBootstrap(t, db, "UPDATE network_profiles SET mode='proxy_required',proxy_id=$2 WHERE account_id=$1", a, ps[0])
			if mode == "direct-proxy-direct" {
				execBootstrap(t, db, "UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1", a)
			}
			if mode == "captured-before-build" {
				rt, err = c.buildProviderNetworkRuntime(ctx, cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer rt.CloseIdleConnections()
			}
			resp, err := rt.Client.Get(server.URL)
			if resp != nil {
				resp.Body.Close()
			}
			if !errors.Is(err, ErrStaleAccountNetwork) || calls.Load() != 0 {
				t.Fatalf("obsolete direct route sent traffic: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
	t.Run("admitted-exchange-blocks-mode-change", func(t *testing.T) {
		db := installerBootstrapDB(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, a, ps := countryFixture(t, db, "")
		execBootstrap(t, db, "UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1", a)
		cfg, err := c.Accounts.Account(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		rt, err := c.buildProviderNetworkRuntime(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer rt.CloseIdleConnections()
		released := make(chan struct{})
		var once sync.Once
		release := func() { once.Do(func() { close(released) }) }
		defer release()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			select {
			case <-released:
			case <-r.Context().Done():
			}
		}))
		defer func() { release(); server.Close() }()
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		resp, err := rt.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		writer := make(chan error, 1)
		go func() {
			tx, e := countryRouteTx(ctx, db, strings.ToUpper(a))
			if e == nil {
				defer tx.Rollback()
				_, e = tx.ExecContext(ctx, "UPDATE network_profiles SET mode='proxy_required',proxy_id=$2 WHERE account_id=$1", a, ps[0])
				if e == nil {
					e = tx.Commit()
				}
			}
			writer <- e
		}()
		select {
		case e := <-writer:
			t.Fatal("mode changed before response completion", e)
		case <-time.After(100 * time.Millisecond):
		}
		release()
		if _, err = io.ReadAll(resp.Body); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		select {
		case err = <-writer:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("mode change never resumed")
		}
	})
}

func TestFundedReviewRejectedBodyCleanupPostgres(t *testing.T) {
	for _, cause := range []string{"stale-route", "lock-canceled", "check-failure", "missing-base"} {
		t.Run(cause, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			base := &countRoundTripper{}
			guard := accountNetworkGuardTransport{Base: base}
			want := ErrNetworkNotReady
			switch cause {
			case "stale-route", "lock-canceled":
				db := installerBootstrapDB(t)
				c, a, ps := countryFixture(t, db, "")
				execBootstrap(t, db, "UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1", a)
				cfg, err := c.Accounts.Account(ctx, a)
				if err != nil {
					t.Fatal(err)
				}
				rt, err := c.buildProviderNetworkRuntime(ctx, cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer rt.CloseIdleConnections()
				var ok bool
				guard, ok = rt.Client.Transport.(accountNetworkGuardTransport)
				if !ok {
					t.Fatal("direct runtime missing guard")
				}
				guard.Base = base
				if cause == "stale-route" {
					execBootstrap(t, db, "UPDATE network_profiles SET mode='proxy_required',proxy_id=$2 WHERE account_id=$1", a, ps[0])
					want = ErrStaleAccountNetwork
				} else {
					cancel()
					want = context.Canceled
				}
			case "check-failure":
				want = errors.New("injected admission failure")
				guard.Check = func(context.Context) error { return want }
			case "missing-base":
				guard.Check = func(context.Context) error { return nil }
				guard.Base = nil
			}
			reader, writer := io.Pipe()
			body := &fundedCloseRecorder{ReadCloser: reader, called: make(chan struct{})}
			// Cleanup uses the underlying reader, so it cannot conceal a missed
			// transport Close or inflate the observed transport close count.
			defer reader.Close()
			defer writer.Close()
			started, produced := make(chan struct{}), make(chan error, 1)
			go func() {
				close(started)
				_, err := writer.Write([]byte("streaming provider payload"))
				produced <- err
			}()
			<-started
			select {
			case err := <-produced:
				t.Fatalf("producer finished before admission: %v", err)
			default:
			}
			req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:1/never-dispatched", body)
			if err != nil {
				t.Fatal(err)
			}
			var response *http.Response
			if cause == "stale-route" {
				response, err = (&http.Client{Transport: guard}).Do(req)
			} else {
				response, err = guard.RoundTrip(req)
			}
			if response != nil || !errors.Is(err, want) {
				t.Fatalf("unexpected rejection: response=%v err=%v want=%v", response, err, want)
			}
			if base.calls.Load() != 0 {
				t.Fatal("rejected request reached base transport")
			}
			if body.count.Load() != 1 {
				t.Errorf("rejected request body closed %d times, want one", body.count.Load())
			}
			select {
			case err := <-produced:
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("producer error=%v", err)
				}
			case <-time.After(time.Second):
				t.Error("rejected streaming producer remains stranded")
			}
		})
	}
}

type fundedCloseRecorder struct {
	io.ReadCloser
	called chan struct{}
	count  atomic.Int32
}

func (b *fundedCloseRecorder) Close() error {
	if b.count.Add(1) == 1 {
		close(b.called)
	}
	return b.ReadCloser.Close()
}

// Complete the HTTP/2 response without resetting the client's request stream.
// The standard HTTP server can send an automatic reset when its handler ends,
// which would conceal the EOF-followed-by-Close cancellation regression.
func fundedHTTP2Peer(t *testing.T, empty bool) (string, func()) {
	t.Helper()
	seed := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cfg := seed.TLS.Clone()
	cfg.NextProtos = []string{"h2"}
	seed.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var accepted net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		mu.Lock()
		accepted = conn
		mu.Unlock()
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
		const clientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
		preface := make([]byte, len(clientPreface))
		if _, e = io.ReadFull(conn, preface); e != nil || string(preface) != clientPreface {
			return
		}
		// A minimal loopback-only peer: SETTINGS, one HEADERS response and DATA.
		// Deliberately never synthesize RST_STREAM when the response completes.
		writeFrame := func(kind, flags byte, stream uint32, payload []byte) error {
			header := make([]byte, 9)
			header[0], header[1], header[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
			header[3], header[4] = kind, flags
			binary.BigEndian.PutUint32(header[5:], stream)
			_, err := io.Copy(conn, bytes.NewReader(append(header, payload...)))
			return err
		}
		if e = writeFrame(4, 0, 0, nil); e != nil {
			return
		}
		replied := false
		for {
			header := make([]byte, 9)
			if _, e = io.ReadFull(conn, header); e != nil {
				return
			}
			length := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
			if length > 1<<20 {
				return
			}
			if _, e = io.CopyN(io.Discard, conn, int64(length)); e != nil {
				return
			}
			stream := binary.BigEndian.Uint32(header[5:]) & 0x7fffffff
			switch header[3] {
			case 4: // SETTINGS
				if header[4]&1 == 0 {
					if writeFrame(4, 1, 0, nil) != nil {
						return
					}
				}
			case 1: // HEADERS
				if !replied {
					var block bytes.Buffer
					encoder := hpack.NewEncoder(&block)
					if encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}) != nil {
						return
					}
					length, flags := "2", byte(4)
					if empty {
						length, flags = "0", 5
					}
					if encoder.WriteField(hpack.HeaderField{Name: "content-length", Value: length}) != nil {
						return
					}
					if writeFrame(1, flags, stream, block.Bytes()) != nil {
						return
					}
					if !empty && writeFrame(0, 1, stream, []byte("ok")) != nil {
						return
					}
					replied = true
				}
			}
		}
	}()
	return "https://" + listener.Addr().String(), func() {
		listener.Close()
		mu.Lock()
		if accepted != nil {
			accepted.Close()
		}
		mu.Unlock()
		<-done
	}
}

func TestFundedReviewNativeEOFThenClosePostgres(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "body"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) { testFundedNativeEOFThenClose(t, empty) })
	}
}
func testFundedNativeEOFThenClose(t *testing.T, empty bool) {
	db := installerBootstrapDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	c, a, ps := countryFixture(t, db, "")
	execBootstrap(t, db, "UPDATE network_profiles SET mode='direct',proxy_id=NULL WHERE account_id=$1", a)
	cfg, err := c.Accounts.Account(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	url, shutdown := fundedHTTP2Peer(t, empty)
	defer shutdown()
	native := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true}
	defer native.CloseIdleConnections()
	body := &delayedNativeUpload{entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(body.release) }) }
	defer release()
	// Keep the native transport and trace callbacks; record actual Close entry.
	var nativeContext context.Context
	var recorded *fundedCloseRecorder
	rt, err := c.buildProviderNetworkRuntime(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.CloseIdleConnections()
	guard, ok := rt.Client.Transport.(accountNetworkGuardTransport)
	if !ok {
		// The pre-fix candidate has no direct guard. Build the same exchange
		// wrapper with a known route check to isolate the separate Close defect.
		guard = accountNetworkGuardTransport{Check: func(context.Context) error { return nil }, NativeWriteTrace: true}
	}
	guard.Base = economyRoundTrip(func(r *http.Request) (*http.Response, error) {
		nativeContext = r.Context()
		response, e := native.RoundTrip(r)
		if e == nil && response.Body != http.NoBody {
			recorded = &fundedCloseRecorder{ReadCloser: response.Body, called: make(chan struct{})}
			response.Body = recorded
		}
		return response, e
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
	resp, err := guard.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ProtoMajor != 2 {
		t.Fatal("native HTTP/2 required")
	}
	select {
	case <-body.entered:
	case <-ctx.Done():
		t.Fatal("native upload never started")
	}
	if _, err = io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	select {
	case <-nativeContext.Done():
		t.Fatal("EOF canceled unfinished upload")
	default:
	}
	closed := make(chan error, 1)
	go func() { closed <- resp.Body.Close() }()
	if recorded != nil {
		select {
		case <-recorded.called:
		case <-time.After(time.Second):
			t.Fatal("EOF consumed explicit native Close")
		}
	}
	select {
	case <-nativeContext.Done():
	case <-time.After(time.Second):
		t.Fatal("explicit Close did not cancel exchange")
	}
	if ok {
		writer := make(chan error, 1)
		go func() {
			tx, e := countryRouteTx(ctx, db, a)
			if e == nil {
				_, e = tx.ExecContext(ctx, "UPDATE network_profiles SET mode='proxy_required',proxy_id=$2 WHERE account_id=$1", a, ps[0])
				if e == nil {
					e = tx.Commit()
				} else {
					tx.Rollback()
				}
			}
			writer <- e
		}()
		select {
		case e := <-writer:
			t.Fatal("Close released route before native write settled", e)
		case <-time.After(80 * time.Millisecond):
		}
		release()
		select {
		case e := <-writer:
			if e != nil {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("route remained pinned after native settlement")
		}
	} else {
		release()
	}
	select {
	case err = <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("native close did not return")
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if recorded != nil && recorded.count.Load() != 1 {
		t.Fatal("native close must run exactly once", recorded.count.Load())
	}
}

func TestFundedReviewOrderedPassiveHealthCASPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, a, ps := countryFixture(t, db, `["US","TR"]`)
	// Establish a healthy TR incumbent with complete higher-tier exhaustion.
	if err := c.maintainOrderedCountryWithProbe(ctx, a, func(_ context.Context, _ countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
		if v.Target == "US" {
			return v, proxyselection.Evaluation{Health: proxyselection.Unusable, ExhaustProvider: true}
		}
		v = countryProof(p.ID, "TR", "198.18.97.5")
		return v, proxyselection.Evaluation{Health: proxyselection.Usable}
	}); err != nil {
		t.Fatal(err)
	}
	execBootstrap(t, db, "UPDATE account_proxy_selection SET checkpoint='{}' WHERE account_id=$1", a)
	s, err := loadCountrySnapshot(ctx, db, a)
	if err != nil {
		t.Fatal(err)
	}
	store := proxycontrol.SQLStore{DB: db}
	before, err := store.Load(ctx, a, s.Active, "digitalocean")
	if err != nil {
		t.Fatal(err)
	}
	paused, resume := make(chan struct{}), make(chan struct{})
	var pauseOnce, resumeOnce sync.Once
	unblock := func() { resumeOnce.Do(func() { close(resume) }) }
	defer unblock()
	result := make(chan error, 1)
	go func() {
		result <- c.maintainOrderedCountryWithProbe(ctx, a, func(_ context.Context, _ countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
			if v.Target == "TR" {
				v = countryProof(p.ID, "TR", s.IP)
				v.Session = s.Session
				return v, proxyselection.Evaluation{Health: proxyselection.Usable}
			}
			pauseOnce.Do(func() { close(paused) })
			select {
			case <-resume:
			case <-ctx.Done():
			}
			return v, proxyselection.Evaluation{Health: proxyselection.Unknown}
		})
	}()
	select {
	case <-paused:
	case <-ctx.Done():
		t.Fatal("higher-country probe never paused")
	}
	var identityBefore string
	if err = db.QueryRow("SELECT row_to_json(i)::text FROM account_network_identities i WHERE account_id=$1", a).Scan(&identityBefore); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_, applied, e := store.ApplyObservationForGeneration(ctx, a, s.Active, "digitalocean", before.Generation, network.HealthResult{Status: network.StatusDown, CheckedAt: time.Now(), Error: "proxy authentication required"}, proxycontrol.DefaultPolicy())
		if e != nil || !applied {
			t.Fatal("passive failure not applied", applied, e)
		}
	}
	unblock()
	select {
	case err = <-result:
	case <-ctx.Done():
		t.Fatal("ordered publication never finished")
	}
	if !errors.Is(err, proxycontrol.ErrStateConflict) {
		t.Fatal("old incumbent proof overwrote passive health", err)
	}
	after, err := store.Load(ctx, a, s.Active, "digitalocean")
	if err != nil {
		t.Fatal(err)
	}
	if after.ConsecutiveFailures != 3 || after.CircuitState != "open" || after.Generation != before.Generation {
		t.Fatal("new passive evidence lost", after)
	}
	var identityAfter string
	if err = db.QueryRow("SELECT row_to_json(i)::text FROM account_network_identities i WHERE account_id=$1", a).Scan(&identityAfter); err != nil || identityAfter != identityBefore {
		t.Fatal("obsolete publication partially changed identity", err)
	}
	// A genuinely new route may replace old-generation health using its own
	// observation timestamp, and old-generation passive results must be ignored.
	next, err := c.claimCountryWork(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	v := countryProof(ps[0], "US", "198.18.98.5")
	v.Session = "fresh-route"
	v.ObservedAt = time.Now().Add(-5 * time.Second).Truncate(time.Microsecond)
	if err = c.finishCountryWork(ctx, next, &v, nil, "ready", "fresh proof", true); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Load(ctx, a, ps[0], "digitalocean")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Generation <= before.Generation || fresh.LastCheckedAt == nil || !fresh.LastCheckedAt.Equal(v.ObservedAt) {
		t.Fatal("new route lacked generation/time fence", fresh)
	}
	_, applied, err := store.ApplyObservationForGeneration(ctx, a, ps[0], "digitalocean", before.Generation, network.HealthResult{Status: network.StatusDown, CheckedAt: time.Now()}, proxycontrol.DefaultPolicy())
	if err != nil || applied {
		t.Fatal("old-generation observation reached new route", applied, err)
	}
}
