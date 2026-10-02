package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
)

var ErrStaleAccountNetwork = errors.New("stale account network runtime")

type accountNetworkGuardTransport struct {
	Base           http.RoundTripper
	Container      Container
	AccountID      string
	Provider       string
	ProxyID        string
	TransportEpoch int64
	Check          func(context.Context) error
}

func (t accountNetworkGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Check != nil {
		if err := t.Check(req.Context()); err != nil {
			return nil, err
		}
	} else {
		if err := t.Container.requireAccountNetworkReadyForProxy(req.Context(), t.AccountID, t.ProxyID); err != nil {
			return nil, err
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
		base = http.DefaultTransport
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
