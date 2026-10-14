package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"os"
	"time"
)

type result struct {
	Account  string `json:"account_id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
	ipam.ReadResult
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	a, e := app.Bootstrap(ctx)
	if e != nil {
		os.Stderr.WriteString("application bootstrap unavailable\n")
		os.Exit(1)
	}
	defer a.Close()
	rows, e := a.DB.QueryContext(ctx, "SELECT id::text,name,provider,enabled FROM accounts WHERE deleted_at IS NULL ORDER BY provider,id")
	if e != nil {
		os.Exit(1)
	}
	xs := []result{}
	for rows.Next() {
		var x result
		if rows.Scan(&x.Account, &x.Name, &x.Provider, &x.Enabled) != nil {
			os.Exit(1)
		}
		xs = append(xs, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	s := ipam.InventoryStore{DB: a.DB}
	failed := false
	for _, x := range xs {
		if !x.Enabled {
			x.State = "ACCOUNT_DISABLED"
		} else {
			x.ReadResult, e = s.Refresh(ctx, x.Account, a.Container.ReadIPInventory)
			if e != nil {
				failed = true
				x.State = "UNCOMMITTED"
				x.ErrorClass = "unavailable"
				if errors.Is(e, ipam.ErrLaneBusy) {
					x.State = "BUSY_OR_COOLDOWN"
					x.ErrorClass = ""
				}
			}
		}
		if x.State == "FAILED" {
			failed = true
		}
		if enc.Encode(x) != nil {
			failed = true
			break
		}
	}
	if failed {
		os.Exit(1)
	}
}
