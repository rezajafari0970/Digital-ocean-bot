package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultrconsole"
)

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func main() {
	if len(os.Args) != 2 {
		panic("account id required")
	}
	accountID := os.Args[1]
	ctx := context.Background()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}
	defer a.Close()

	var email, passRef, proxyID, proxyHost, proxyUser, proxyRef string
	var proxyPort int
	err = a.DB.QueryRowContext(ctx, `
SELECT COALESCE(ac.console_email,''),COALESCE(ac.console_password_secret_ref,''),
       COALESCE(p.id::text,''),COALESCE(p.host,''),COALESCE(p.port,0),COALESCE(p.username,''),COALESCE(p.secret_ref,'')
FROM accounts ac
LEFT JOIN account_proxy_pool ap ON ap.account_id=ac.id AND ap.enabled=true AND ap.priority=1
LEFT JOIN proxies p ON p.id=ap.proxy_id
WHERE ac.id=$1 AND ac.provider='vultr'`, accountID).
		Scan(&email, &passRef, &proxyID, &proxyHost, &proxyPort, &proxyUser, &proxyRef)
	if err != nil {
		panic(err)
	}
	if email == "" || passRef == "" {
		panic("console credentials not configured")
	}
	pass, err := a.Container.Secrets.Get(ctx, accountID, passRef)
	if err != nil {
		panic(err)
	}
	defer wipe(pass)

	proxyURL := ""
	if proxyHost != "" {
		u := &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", proxyHost, proxyPort)}
		if proxyRef != "" {
			pp, e := a.Container.Secrets.GetProxy(ctx, proxyID, proxyRef)
			if e == nil {
				defer wipe(pp)
				u.User = url.UserPassword(proxyUser, string(pp))
			}
		}
		proxyURL = u.String()
	}
	if proxyURL != "" {
		u, e := url.Parse(proxyURL)
		if e != nil {
			panic(e)
		}
		bridge := &vultrconsole.ProxyBridge{Upstream: u}
		localProxy, e := bridge.Start()
		if e != nil {
			panic(e)
		}
		defer bridge.Close()
		proxyURL = localProxy
	}
	observer := vultrconsole.BrowserObserver{ChromePath: "/usr/bin/google-chrome-stable", Timeout: 60 * time.Second}
	limit, err := observer.Observe(ctx, email, string(pass), proxyURL)
	if errors.Is(err, vultrconsole.ErrChallenge) {
		_, _ = a.DB.ExecContext(ctx, `UPDATE accounts SET console_capacity_status='challenge_required',console_capacity_checked_at=now(),console_capacity_detail='manual challenge required' WHERE id=$1`, accountID)
		fmt.Println("challenge_required")
		return
	}
	if err != nil {
		_, _ = a.DB.ExecContext(ctx, `UPDATE accounts SET console_capacity_status='failed',console_capacity_checked_at=now(),console_capacity_detail=$2 WHERE id=$1`, accountID, err.Error())
		fmt.Println("failed")
		return
	}
	_, err = a.DB.ExecContext(ctx, `
INSERT INTO provider_capacity_observations(account_id,compute_limit,source,observed_at,updated_at)
VALUES($1,$2,'vultr_console',now(),now())
ON CONFLICT(account_id) DO UPDATE SET compute_limit=EXCLUDED.compute_limit,source=EXCLUDED.source,observed_at=now(),updated_at=now()`, accountID, limit)
	if err != nil {
		panic(err)
	}
	_, _ = a.DB.ExecContext(ctx, `UPDATE accounts SET console_capacity_status='healthy',console_capacity_checked_at=now(),console_capacity_detail='maximum instances observed',provider_checked_at='epoch'::timestamptz WHERE id=$1`, accountID)
	fmt.Printf("healthy limit=%d\n", limit)
}
