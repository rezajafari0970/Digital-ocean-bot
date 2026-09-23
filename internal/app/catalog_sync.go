package app

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

func (c Container) SyncCatalogs(ctx context.Context) {
	rows, err := c.DB.QueryContext(ctx, `SELECT id::text FROM accounts WHERE enabled=true AND provider='digitalocean'`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		rt, err := c.Runtime(ctx, id)
		if err != nil {
			log.Printf("catalog sync %s runtime: %v", id, err)
			continue
		}
		d, err := rt.Provider.Discover(ctx)
		if rt.Gateway != nil {
			rt.Gateway.CloseIdleConnections()
		}
		if err != nil {
			log.Printf("catalog sync %s discovery: %v", id, err)
			continue
		}
		b, _ := json.Marshal(d)
		_, err = c.DB.ExecContext(ctx, `INSERT INTO provider_catalog_cache(account_id,catalog,refreshed_at) VALUES($1,$2,now()) ON CONFLICT(account_id) DO UPDATE SET catalog=EXCLUDED.catalog,refreshed_at=now()`, id, b)
		if err != nil {
			log.Printf("catalog sync %s save: %v", id, err)
		}
	}
}
func (c Container) RunDailyCatalogSync(ctx context.Context) {
	c.SyncCatalogs(ctx)
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.SyncCatalogs(ctx)
		}
	}
}
