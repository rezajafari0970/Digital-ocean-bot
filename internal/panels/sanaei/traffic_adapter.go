package sanaei

import "context"

type TrafficAdapter struct{ API *APIClient }

func (a TrafficAdapter) Get(ctx context.Context, email string) (int64, int64, error) {
	t, err := a.API.GetClientTraffic(ctx, email)
	return t.Up, t.Down, err
}

type ClientDisabler struct {
	API       *APIClient
	Store     *SQLClientStore
	AccountID string
	InboundID int
}

func (d ClientDisabler) Disable(ctx context.Context, clientID string) error {
	if err := d.API.DeleteClient(ctx, d.InboundID, clientID); err != nil {
		return err
	}
	if d.Store != nil {
		return d.Store.SetEnabled(ctx, d.AccountID, clientID, false)
	}
	return nil
}
