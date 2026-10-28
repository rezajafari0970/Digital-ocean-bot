package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

var ErrStaleAccountNetwork = errors.New("stale account network runtime")

func (c Container) acquireAccountRouteSharedLock(ctx context.Context, accountID string) (*sql.Conn, error) {
	if c.DB == nil {
		return nil, ErrNetworkNotReady
	}
	conn, err := c.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1,0))`, "account-route:"+accountID); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func releaseAccountRouteSharedLock(conn *sql.Conn, accountID string) {
	if conn == nil {
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
	Base           http.RoundTripper
	Container      Container
	AccountID      string
	Provider       string
	ProxyID        string
	Generation     int64
	TransportEpoch int64
	StickySession  string
	ExitIP         string
	Check          func(context.Context) error
	Observe        network.TransportObserver
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
		exchange.uploadDone = true
	} else {
		body := &guardBody{ReadCloser: req.Body, done: func() { exchange.complete(true) }, closeOnEOF: false}
		exchange.requestBody = body
		copy.Body = body
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
	if resp == nil || resp.Body == nil || resp.Body == http.NoBody {
		exchange.responseDone = true
	} else {
		body := &guardBody{ReadCloser: resp.Body, done: func() { exchange.complete(false) }, closeOnEOF: true, beforeClose: cancel}
		exchange.responseBody = body
		resp.Body = body
	}
	exchange.mu.Unlock()
	if err != nil || ctx.Err() != nil {
		exchange.abort()
	} else {
		exchange.maybeFinish()
	}
	return resp, err
}

func (t accountNetworkGuardTransport) checkConnection(ctx context.Context, q countryQuery) error {
	if err := requireAccountNetworkReadyQuery(ctx, q, t.AccountID, t.Container.Economy != nil); err != nil {
		return err
	}
	var proxy, session, ip string
	var generation, epoch int64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(n.proxy_id::text,''),COALESCE(i.sticky_session,''),COALESCE(host(i.exit_ip),''),r.generation,e.transport_epoch
 FROM network_profiles n LEFT JOIN account_network_identities i ON i.account_id=n.account_id
 JOIN proxy_runtime_state r ON r.account_id=n.account_id AND r.proxy_id=n.proxy_id AND r.provider=$2
 JOIN account_transport_state e ON e.account_id=n.account_id AND e.provider=$2
 WHERE n.account_id=$1`, t.AccountID, t.Provider).Scan(&proxy, &session, &ip, &generation, &epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleAccountNetwork
	}
	if err != nil {
		return err
	}
	if proxy != t.ProxyID || session != t.StickySession || ip != t.ExitIP || generation != t.Generation || epoch != t.TransportEpoch {
		return ErrStaleAccountNetwork
	}
	return nil
}

type guardExchange struct {
	mu                                              sync.Mutex
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
	once        sync.Once
	done        func()
	beforeClose func()
	closeOnEOF  bool
}

func (b *guardBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && b.closeOnEOF {
		b.Close()
	}
	return n, err
}
func (b *guardBody) Close() error {
	var err error
	b.once.Do(func() {
		if b.beforeClose != nil {
			b.beforeClose()
		}
		err = b.ReadCloser.Close()
		b.done()
	})
	return err
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
