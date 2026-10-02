package app

import (
    "context"
    "database/sql"
    "os"
    "testing"

    _ "github.com/lib/pq"
)

func TestProxyPoolSwitchBumpsEpochAtomicallyPostgresE2E(t *testing.T) {
    dsn := os.Getenv("DOB_E2E_DSN")
    if dsn == "" { t.Skip("DOB_E2E_DSN not set") }
    db, err := sql.Open("postgres", dsn)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    ctx := context.Background()

    var accountID, p1, p2 string
    if err := db.QueryRowContext(ctx, `INSERT INTO accounts(id,provider,name,secret_ref)
VALUES(gen_random_uuid(),'digitalocean','pool-e2e-'||gen_random_uuid()::text,'x') RETURNING id::text`).Scan(&accountID); err != nil { t.Fatal(err) }
    defer db.ExecContext(context.Background(), "DELETE FROM accounts WHERE id=$1", accountID)

    if err := db.QueryRowContext(ctx, `INSERT INTO proxies(id,name,type,host,port,status)
VALUES(gen_random_uuid(),'pool-p1-'||gen_random_uuid()::text,'http','127.0.0.1',18080,'down') RETURNING id::text`).Scan(&p1); err != nil { t.Fatal(err) }
    defer db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", p1)
    if err := db.QueryRowContext(ctx, `INSERT INTO proxies(id,name,type,host,port,status)
VALUES(gen_random_uuid(),'pool-p2-'||gen_random_uuid()::text,'http','127.0.0.1',18081,'healthy') RETURNING id::text`).Scan(&p2); err != nil { t.Fatal(err) }
    defer db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", p2)
    if _, err := db.ExecContext(ctx, `INSERT INTO network_profiles(id,account_id,mode,proxy_id)
VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, accountID, p1); err != nil { t.Fatal(err) }
    if _, err := db.ExecContext(ctx, `INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled)
VALUES($1,$2,0,true),($1,$3,10,true)`, accountID, p1, p2); err != nil { t.Fatal(err) }
    if _, err := db.ExecContext(ctx, `INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
VALUES($1,'digitalocean',7,$2,'before-test')`, accountID, p1); err != nil { t.Fatal(err) }
    if _, err := db.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,timezone,locale,exit_ip,subnet_key,preferred_country,preferred_country_code,last_health_ok)
VALUES($1,'Europe/Berlin','de-DE','203.0.113.210','203.0.113.0/24','Germany','de',true)`, accountID); err != nil { t.Fatal(err) }

    c := Container{DB:db}
    if err := c.ensureActiveAccountProxy(ctx, accountID); err != nil { t.Fatal(err) }

    var active string
    var epoch int64
    var transportProxy, country, cc string
    var health bool
    var rotating bool
    if err := db.QueryRowContext(ctx, `SELECT np.proxy_id::text,ats.transport_epoch,ats.active_proxy_id::text,
COALESCE(ani.preferred_country,''),COALESCE(ani.preferred_country_code,''),COALESCE(ani.last_health_ok,false),
ani.rotation_started_at IS NOT NULL
FROM network_profiles np
JOIN account_transport_state ats ON ats.account_id=np.account_id
JOIN account_network_identities ani ON ani.account_id=np.account_id
WHERE np.account_id=$1`, accountID).Scan(&active,&epoch,&transportProxy,&country,&cc,&health,&rotating); err != nil { t.Fatal(err) }
    if active != p2 || transportProxy != p2 {
        t.Fatalf("active=%s transport=%s want=%s", active, transportProxy, p2)
    }
    if epoch != 8 { t.Fatalf("epoch=%d want=8", epoch) }
    if country != "Germany" || cc != "de" {
        t.Fatalf("preferred country lost: %q %q", country, cc)
    }
    if health || !rotating {
        t.Fatalf("identity must be fail-closed during backup recovery: health=%v rotating=%v", health, rotating)
    }
}
