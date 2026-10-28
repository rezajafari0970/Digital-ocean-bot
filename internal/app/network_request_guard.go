package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

var ErrStaleAccountNetwork = errors.New("stale account network runtime")

func (c Container) acquireAccountRouteSharedLock(ctx context.Context, accountID string) (*sql.Conn, error) {
	if c.DB == nil {
		return nil, ErrNetworkNotReady
	}
	accountID, err := network.CanonicalAccountID(accountID)
	if err != nil {
		return nil, err
	}
	conn, err := c.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1,0))`, "account-route:"+accountID); err != nil {
		// An interrupted acknowledgement cannot prove the session lock was not acquired.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func releaseAccountRouteSharedLock(conn *sql.Conn, accountID string) {
	if conn == nil {
		return
	}
	accountID, err := network.CanonicalAccountID(accountID)
	if err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		conn.Close()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock_shared(hashtextextended($1,0))`, "account-route:"+accountID); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	_ = conn.Close()
}

type accountNetworkGuardTransport struct {
	Base             http.RoundTripper
	Container        Container
	AccountID        string
	Provider         string
	ProxyID          string
	Generation       int64
	TransportEpoch   int64
	StickySession    string
	ExitIP           string
	RouteFingerprint string
	NativeWriteTrace bool
	Direct           bool
	Check            func(context.Context) error
	Observe          network.TransportObserver
}

// The route remains pinned for the complete exchange, including request-body
// upload after early HTTP/2 response headers. Observers run only after release.
func (t accountNetworkGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var conn *sql.Conn
	if t.Check == nil {
		var err error
		conn, err = t.Container.acquireAccountRouteSharedLock(req.Context(), t.AccountID)
		if err != nil {
			return nil, err
		}
		if err = t.checkConnection(req.Context(), conn); err != nil {
			releaseAccountRouteSharedLock(conn, t.AccountID)
			return nil, err
		}
	} else if err := t.Check(req.Context()); err != nil {
		return nil, err
	}
	if t.Base == nil {
		releaseAccountRouteSharedLock(conn, t.AccountID)
		return nil, ErrNetworkNotReady
	}
	ctx, cancel := context.WithCancel(req.Context())
	copy := req.Clone(ctx)
	exchange := &guardExchange{release: func() { releaseAccountRouteSharedLock(conn, t.AccountID) }, observe: t.Observe, cancel: cancel}
	if req.Body == nil || req.Body == http.NoBody {
		exchange.uploadDone = !t.NativeWriteTrace
	} else {
		body := &guardBody{ReadCloser: req.Body, done: func() {
			if !t.NativeWriteTrace {
				exchange.complete(true)
			}
		}, closeOnEOF: false}
		exchange.requestBody = body
		copy.Body = body
	}
	if t.NativeWriteTrace {
		copy = copy.WithContext(httptrace.WithClientTrace(copy.Context(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				exchange.mu.Lock()
				exchange.nativeConn = info.Conn
				exchange.uploadDone = false
				exchange.wroteHeaders = false
				exchange.mu.Unlock()
			},
			WroteHeaders: func() { exchange.mu.Lock(); exchange.wroteHeaders = true; exchange.mu.Unlock() },
			WroteRequest: func(httptrace.WroteRequestInfo) { exchange.complete(true) },
		}))
	}
	stop := context.AfterFunc(ctx, exchange.abort)
	defer func() {
		exchange.mu.Lock()
		exchange.stop = stop
		finished := exchange.finished
		exchange.mu.Unlock()
		if finished {
			stop()
		}
	}()
	started := time.Now()
	resp, err := t.Base.RoundTrip(copy)
	exchange.mu.Lock()
	exchange.observation = network.TransportObservation{StartedAt: started, Latency: time.Since(started), Err: err, ProxyAuthRequired: resp != nil && resp.StatusCode == http.StatusProxyAuthRequired}
	exchange.headersDone = true
	if resp == nil || resp.Body == nil {
		exchange.responseDone = true
	} else {
		// An explicitly empty response is already drained, but its caller still
		// owns Close as a way to cancel an unfinished native request upload.
		exchange.responseDone = resp.Body == http.NoBody
		body := &guardBody{ReadCloser: resp.Body, done: func() { exchange.complete(false) }, closeOnEOF: true, beforeClose: cancel}
		exchange.responseBody = body
		resp.Body = body
	}
	exchange.mu.Unlock()
	if t.NativeWriteTrace && (err != nil || ctx.Err() != nil) {
		exchange.settleBeforeHeaders()
	}
	if err != nil || ctx.Err() != nil {
		exchange.abort()
	} else {
		exchange.maybeFinish()
	}
	return resp, err
}

func (t accountNetworkGuardTransport) checkConnection(ctx context.Context, q countryQuery) error {
	if t.Direct {
		var mode, proxy, provider, fingerprint string
		var epoch int64
		err := q.QueryRowContext(ctx, `SELECT n.mode,COALESCE(n.proxy_id::text,''),a.provider,
 COALESCE(e.transport_epoch,0),`+routeFingerprintSQL+`
 FROM accounts a JOIN network_profiles n ON n.account_id=a.id
 LEFT JOIN proxies p ON p.id=n.proxy_id LEFT JOIN account_network_identities i ON i.account_id=a.id
 LEFT JOIN account_transport_state e ON e.account_id=a.id AND e.provider=a.provider
 WHERE a.id=$1`, t.AccountID).Scan(&mode, &proxy, &provider, &epoch, &fingerprint)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStaleAccountNetwork
		}
		if err != nil {
			return err
		}
		if mode != string(network.RouteDirect) || proxy != "" || provider != t.Provider ||
			epoch != t.TransportEpoch || t.RouteFingerprint == "" || fingerprint != t.RouteFingerprint {
			return ErrStaleAccountNetwork
		}
		return nil
	}
	if err := requireAccountNetworkReadyQuery(ctx, q, t.AccountID, t.Container.Economy != nil); err != nil {
		return err
	}
	var proxy, session, ip, fingerprint string
	var generation, epoch int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(n.proxy_id::text,''),COALESCE(i.sticky_session,''),COALESCE(host(i.exit_ip),''),r.generation,e.transport_epoch,`+routeFingerprintSQL+`
 FROM network_profiles n JOIN proxies p ON p.id=n.proxy_id LEFT JOIN account_network_identities i ON i.account_id=n.account_id
 JOIN proxy_runtime_state r ON r.account_id=n.account_id AND r.proxy_id=n.proxy_id AND r.provider=$2
 JOIN account_transport_state e ON e.account_id=n.account_id AND e.provider=$2
 WHERE n.account_id=$1`, t.AccountID, t.Provider).Scan(&proxy, &session, &ip, &generation, &epoch, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleAccountNetwork
	}
	if err != nil {
		return err
	}
	if (t.RouteFingerprint != "" && fingerprint != t.RouteFingerprint) || proxy != t.ProxyID || session != t.StickySession || ip != t.ExitIP || generation != t.Generation || epoch != t.TransportEpoch {
		return ErrStaleAccountNetwork
	}
	return nil
}

type guardExchange struct {
	mu                                              sync.Mutex
	nativeConn                                      net.Conn
	wroteHeaders                                    bool
	uploadDone, responseDone, headersDone, finished bool
	requestBody, responseBody                       io.Closer
	release                                         func()
	cancel                                          context.CancelFunc
	stop                                            func() bool
	observe                                         network.TransportObserver
	observation                                     network.TransportObservation
}

func (e *guardExchange) complete(upload bool) {
	e.mu.Lock()
	if upload {
		e.uploadDone = true
	} else {
		e.responseDone = true
	}
	e.mu.Unlock()
	e.maybeFinish()
}
func (e *guardExchange) maybeFinish() {
	e.mu.Lock()
	if e.finished || !e.uploadDone || !e.responseDone || !e.headersDone {
		e.mu.Unlock()
		return
	}
	e.finished = true
	stop := e.stop
	obs := e.observation
	e.mu.Unlock()
	if stop != nil {
		stop()
	}
	e.release()
	e.cancel()
	if e.observe != nil {
		e.observe(obs)
	}
}
func (e *guardExchange) settleBeforeHeaders() {
	e.mu.Lock()
	pending := !e.uploadDone && !e.wroteHeaders
	conn := e.nativeConn
	e.mu.Unlock()
	if !pending {
		return
	}
	if conn != nil {
		_ = conn.Close()
	}
	e.complete(true)
}
func (e *guardExchange) abort() {
	e.mu.Lock()
	request, response := e.requestBody, e.responseBody
	e.mu.Unlock()
	if request != nil {
		request.Close()
	}
	if response != nil {
		response.Close()
	}
	e.maybeFinish()
}

type guardBody struct {
	io.ReadCloser
	doneOnce    sync.Once
	closeOnce   sync.Once
	closeErr    error
	done        func()
	beforeClose func()
	closeOnEOF  bool
}

func (b *guardBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF && b.closeOnEOF {
		// Native HTTP/2 Body.Close may cancel or wait for the still-active upload.
		// EOF already drained the response; only mark this half complete.
		b.doneOnce.Do(b.done)
	} else if err != nil && b.closeOnEOF {
		b.finish(true)
	}
	return n, err
}
func (b *guardBody) Close() error { return b.finish(true) }
func (b *guardBody) finish(abort bool) error {
	b.closeOnce.Do(func() {
		if abort && b.beforeClose != nil {
			b.beforeClose()
		}
		b.closeErr = b.ReadCloser.Close()
		b.doneOnce.Do(b.done)
	})
	return b.closeErr
}

func (c Container) requireAccountNetworkReadyForProxy(ctx context.Context, accountID, proxyID string) error {
	if err := c.requireAccountNetworkReady(ctx, accountID); err != nil {
		return err
	}
	var current string
	if err := c.DB.QueryRowContext(ctx, `SELECT COALESCE(proxy_id::text,'') FROM network_profiles WHERE account_id=$1`, accountID).Scan(&current); err != nil {
		return err
	}
	if current == "" || current != proxyID {
		return ErrStaleAccountNetwork
	}
	return nil
}
