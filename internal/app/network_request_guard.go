package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
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
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock_shared(hashtextextended($1,0))`, "account-route:"+accountID)
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
}

func (t accountNetworkGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var routeConn *sql.Conn
	if t.Check == nil {
		var err error
		routeConn, err = t.Container.acquireAccountRouteSharedLock(req.Context(), t.AccountID)
		if err != nil {
			return nil, err
		}
		defer releaseAccountRouteSharedLock(routeConn, t.AccountID)
	}
	if t.Check != nil {
		if err := t.Check(req.Context()); err != nil {
			return nil, err
		}
	} else {
		if err := t.Container.requireAccountNetworkReadyForProxy(req.Context(), t.AccountID, t.ProxyID); err != nil {
			return nil, err
		}
		currentSession, currentIP := proxyIdentitySignature(req.Context(), t.Container.DB, t.AccountID)
		if currentSession != t.StickySession || currentIP != t.ExitIP {
			return nil, ErrStaleAccountNetwork
		}
		generation, found, err := (proxycontrol.SQLStore{DB: t.Container.DB}).CurrentGeneration(req.Context(), t.AccountID, t.ProxyID, t.Provider)
		if err != nil {
			return nil, err
		}
		if !found || generation != t.Generation {
			return nil, ErrStaleAccountNetwork
		}
		epoch, ok, err := (proxycontrol.TransportEpochStore{DB: t.Container.DB}).CurrentEpoch(req.Context(), t.AccountID, t.Provider)
		if err != nil {
			return nil, err
		}
		if !ok || epoch != t.TransportEpoch {
			return nil, ErrStaleAccountNetwork
		}
	}
	base := t.Base
	if base == nil {
		return nil, ErrNetworkNotReady
	}
	return base.RoundTrip(req)
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
