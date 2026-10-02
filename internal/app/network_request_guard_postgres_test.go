package app

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	_ "github.com/lib/pq"
)

type countRoundTripper struct{ calls atomic.Int32 }

func (r *countRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return &http.Response{StatusCode: 204, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestAccountNetworkGuardRejectsReassignmentBeforeNetworkPostgresE2E(t *testing.T) {
	dsn := os.Getenv("DOB_E2E_DSN")
	if dsn == "" {
		t.Skip("DOB_E2E_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var accountID, p1, p2 string
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','guard-e2e','x') RETURNING id::text`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM accounts WHERE id=$1", accountID)
	if err := db.QueryRowContext(ctx, `INSERT INTO proxies(id,name,type,host,port,status) VALUES(gen_random_uuid(),'guard-e2e-p1','http','127.0.0.1',18080,'healthy') RETURNING id::text`).Scan(&p1); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", p1)
	if err := db.QueryRowContext(ctx, `INSERT INTO proxies(id,name,type,host,port,status) VALUES(gen_random_uuid(),'guard-e2e-p2','http','127.0.0.1',18081,'healthy') RETURNING id::text`).Scan(&p2); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM proxies WHERE id=$1", p2)

	if _, err := db.ExecContext(ctx, `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)`, accountID, p1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,timezone,locale,exit_ip,subnet_key,last_health_ok) VALUES($1,'UTC','en-US','203.0.113.211','203.0.113.0/24',true)`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason) VALUES($1,'digitalocean',1,$2,'test')`, accountID, p1); err != nil {
		t.Fatal(err)
	}

	base := &countRoundTripper{}
	guard := accountNetworkGuardTransport{Base: base, Container: Container{DB: db}, AccountID: accountID, Provider: "digitalocean", ProxyID: p1, TransportEpoch: 1}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/", nil)
	if _, err := guard.RoundTrip(req); err != nil {
		t.Fatalf("initial guard: %v", err)
	}
	if base.calls.Load() != 1 {
		t.Fatalf("initial calls=%d", base.calls.Load())
	}

	if _, err := db.ExecContext(ctx, `UPDATE network_profiles SET proxy_id=$2 WHERE account_id=$1`, accountID, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.RoundTrip(req); err == nil {
		t.Fatal("stale runtime request was admitted")
	}
	if base.calls.Load() != 1 {
		t.Fatalf("stale request reached network; calls=%d", base.calls.Load())
	}
}
