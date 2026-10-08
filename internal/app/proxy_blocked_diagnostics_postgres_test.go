package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBlockedBaseProxyDiagnosticsPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	var account, proxy, other string
	row := func(q string, out *string, args ...any) {
		t.Helper()
		if err := db.QueryRow(q, args...).Scan(out); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	row("INSERT INTO accounts(id,provider,name,secret_ref,enabled) VALUES(gen_random_uuid(),'digitalocean','blocked-proxy-fixture','unused',true) RETURNING id::text", &account)
	for _, id := range []*string{&proxy, &other} {
		row("INSERT INTO proxies(id,name,type,host,port,status,health_error,last_checked_at) VALUES(gen_random_uuid(),'blocked-fixture-'||gen_random_uuid()::text,'socks5','127.0.0.1',1,'down','PROXY_AUTH_FAILED',now()) RETURNING id::text", id)
	}
	exec("INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)", account, proxy)
	exec("INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled) VALUES($1,$2,1,true),($1,$3,2,true)", account, proxy, other)
	exec("INSERT INTO account_network_identities(account_id,sticky_session,timezone,locale,last_health_ok) VALUES($1,'KEEP_SESSION','UTC','en-US',false)", account)
	exec(`INSERT INTO proxy_runtime_state(account_id,proxy_id,provider,health_state,circuit_state,generation,consecutive_failures,consecutive_successes,retry_after,last_error_class,last_error_detail,last_checked_at,half_open_probe_in_flight,half_open_probe_lease_until)
 VALUES($1,$2,'digitalocean','degraded','open',7,5,1,now()+interval '2 minutes','ISOLATION_WAIT','old isolation',now()-interval '1 minute',true,now()+interval '1 minute')`, account, proxy)
	invariant := func() string {
		t.Helper()
		var out string
		row(`SELECT jsonb_build_object(
   'runtime',to_jsonb(pr)-'last_error_class'-'last_error_detail'-'last_checked_at'-'updated_at',
   'identity',to_jsonb(ni),'profile',to_jsonb(np))::text
  FROM proxy_runtime_state pr JOIN account_network_identities ni ON ni.account_id=pr.account_id
  JOIN network_profiles np ON np.account_id=pr.account_id
  WHERE pr.account_id=$1 AND pr.proxy_id=$2`, &out, account, proxy)
		return out
	}
	before := invariant()
	if err := c.MaintainProxyControlPlane(ctx, account); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatalf("must remain blocked: %v", err)
	}
	if got := invariant(); got != before {
		t.Fatal("diagnostic update modified admission/identity/route fields")
	}
	var code, detail string
	var at, baseAt time.Time
	if err := db.QueryRow("SELECT r.last_error_class,r.last_error_detail,r.last_checked_at,p.last_checked_at FROM proxy_runtime_state r JOIN proxies p ON p.id=r.proxy_id WHERE r.account_id=$1 AND r.proxy_id=$2", account, proxy).Scan(&code, &detail, &at, &baseAt); err != nil {
		t.Fatal(err)
	}
	if code != "BASE_PROXY_AUTH_FAILED" || !strings.Contains(detail, "identity check skipped") || !at.Equal(baseAt) {
		t.Fatalf("code=%q detail=%q wrong evidence=%v", code, detail, !at.Equal(baseAt))
	}
	reset := func() {
		exec("UPDATE proxy_runtime_state SET last_error_class='KEEP_NEWER',last_error_detail='keep',last_checked_at=now()-interval '1 minute' WHERE account_id=$1 AND proxy_id=$2", account, proxy)
	}
	unchanged := func(t *testing.T) {
		t.Helper()
		if err := c.recordBlockedBaseProxy(ctx, account); err != nil {
			t.Fatal(err)
		}
		var got string
		row("SELECT last_error_class FROM proxy_runtime_state WHERE account_id=$1 AND proxy_id=$2", &got, account, proxy)
		if got != "KEEP_NEWER" {
			t.Fatalf("excluded diagnostic overwritten: %q", got)
		}
	}
	t.Run("equal-observation-preserved", func(t *testing.T) {
		reset()
		exec("UPDATE proxy_runtime_state SET last_checked_at=(SELECT last_checked_at FROM proxies WHERE id=$2) WHERE account_id=$1 AND proxy_id=$2", account, proxy)
		unchanged(t)
	})
	t.Run("newer-observation-wins", func(t *testing.T) {
		reset()
		exec("UPDATE proxy_runtime_state SET last_checked_at=now()+interval '1 minute' WHERE account_id=$1 AND proxy_id=$2", account, proxy)
		unchanged(t)
	})
	t.Run("healthy-current-proxy", func(t *testing.T) {
		reset()
		exec("UPDATE proxies SET status='healthy' WHERE id=$1", proxy)
		unchanged(t)
		exec("UPDATE proxies SET status='down' WHERE id=$1", proxy)
	})
	t.Run("healthy-alternate-exists", func(t *testing.T) {
		reset()
		exec("UPDATE proxies SET status='healthy' WHERE id=$1", other)
		unchanged(t)
		exec("UPDATE proxies SET status='down' WHERE id=$1", other)
	})
	t.Run("disabled-selected-proxy", func(t *testing.T) {
		reset()
		exec("UPDATE account_proxy_pool SET enabled=false WHERE account_id=$1 AND proxy_id=$2", account, proxy)
		unchanged(t)
		exec("UPDATE account_proxy_pool SET enabled=true WHERE account_id=$1 AND proxy_id=$2", account, proxy)
	})
	t.Run("disabled-account", func(t *testing.T) {
		reset()
		exec("UPDATE accounts SET enabled=false WHERE id=$1", account)
		unchanged(t)
		exec("UPDATE accounts SET enabled=true WHERE id=$1", account)
	})
	t.Run("route-switched", func(t *testing.T) {
		reset()
		exec("UPDATE network_profiles SET proxy_id=$2 WHERE account_id=$1", account, other)
		unchanged(t)
		var count int
		if err := db.QueryRow("SELECT count(*) FROM proxy_runtime_state WHERE account_id=$1 AND proxy_id=$2 AND last_error_class='BASE_PROXY_AUTH_FAILED'", account, other).Scan(&count); err != nil || count != 1 {
			t.Fatalf("new route diagnostic missing: %v count=%d", err, count)
		}
		exec("UPDATE network_profiles SET proxy_id=$2 WHERE account_id=$1", account, proxy)
	})
	t.Run("unsafe-base-text-normalized", func(t *testing.T) {
		reset()
		exec("UPDATE proxies SET health_error='URL_SECRET_SENTINEL',last_checked_at=now() WHERE id=$1", proxy)
		if err := c.recordBlockedBaseProxy(ctx, account); err != nil {
			t.Fatal(err)
		}
		var got string
		row("SELECT last_error_class||':'||last_error_detail FROM proxy_runtime_state WHERE account_id=$1 AND proxy_id=$2", &got, account, proxy)
		if got != "BASE_PROXY_UNAVAILABLE:no healthy configured proxy route; identity check skipped" {
			t.Fatal("unsafe base diagnostic copied")
		}
	})
	t.Run("canceled-persistence-visible", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if err := c.recordBlockedBaseProxy(canceled, account); err == nil || errors.Is(err, sql.ErrNoRows) {
			t.Fatal("canceled write error lost")
		}
	})
}
