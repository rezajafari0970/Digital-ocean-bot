package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
)

func TestIPNativeDeleteTargets(t *testing.T) {
	cases := []struct{ method, url, want string }{
		{"DELETE", "https://api.digitalocean.com/v2/droplets/123", "123"},
		{"DELETE", "https://api.vultr.com/v2/instances/abc", "abc"},
		{"DELETE", "https://api.linode.com/v4/linode/instances/12", "12"},
		{"DELETE", "https://api.upcloud.com/1.3/server/abc", "abc"},
		{"POST", "https://api.upcloud.com/1.3/server/abc/stop", "abc"},
		{"GET", "https://api.digitalocean.com/v2/droplets/123", ""},
		{"DELETE", "https://api.digitalocean.com/v2/account/keys/123", ""},
		{"DELETE", "https://unrelated.invalid/v2/droplets/123", ""},
		{"DELETE", "https://api.upcloud.com/1.3/server/abc/storage/attach", ""},
	}
	for _, c := range cases {
		r, _ := http.NewRequest(c.method, c.url, nil)
		if got := nativeIPDeleteTarget(r); got != c.want {
			t.Fatal(c, got)
		}
	}
}
func TestIPNativeDeleteAndPurgePreserveUnresolvedIntent(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	ctx := context.Background()
	var server string
	if e := db.QueryRow("SELECT provider_resource_id FROM droplets WHERE account_id=$1 LIMIT 1", f.d.AccountID).Scan(&server); e != nil {
		t.Fatal(e)
	}
	digest := strings.Repeat("a", 64)
	execBootstrap(t, db, "INSERT INTO ip_plans(digest,account_id,family,policy_revision,body,created_at) VALUES($1,$2,4,1,'{}',now())", digest, f.d.AccountID)
	var approval string
	e := db.QueryRow(`INSERT INTO ip_approvals(plan_digest,account_id,family,approved_by,expires_at,currency,max_commitment_micros,funded_until,max_allocations)
 VALUES($1,$2,4,'fixture',now()+interval '1 hour','USD',1000,now()+interval '1 day',1) RETURNING id::text`, digest, f.d.AccountID).Scan(&approval)
	if e != nil {
		t.Fatal(e)
	}
	execBootstrap(t, db, `INSERT INTO ip_mutation_intents(account_id,provider,family,product,server_id,region,idempotency_key,action,approval_id,request,commitment_micros,state,dispatch_id,dispatched_at)
 VALUES($1,'digitalocean',4,'reserved',$2,'r','fixture','ALLOCATE',$3,'{}',1000,'UNKNOWN',gen_random_uuid(),now())`, f.d.AccountID, server, approval)
	calls := 0
	transport := providerMutationFence{db: db, account: f.d.AccountID, base: fenceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})}
	req, _ := http.NewRequest("DELETE", "https://api.digitalocean.com/v2/droplets/"+server, nil)
	if _, e = transport.RoundTrip(req); !errors.Is(e, ipam.ErrProtected) || calls != 0 {
		t.Fatal("delete reached provider", e, calls)
	}
	req, _ = http.NewRequest("DELETE", "https://api.digitalocean.com/v2/droplets/unrelated-server", nil)
	resp, e := transport.RoundTrip(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	var tombstone bool
	if e = db.QueryRow("SELECT EXISTS(SELECT 1 FROM ip_native_delete_fences WHERE account_id=$1 AND server_id='unrelated-server')", f.d.AccountID).Scan(&tombstone); e != nil || !tombstone {
		t.Fatal("late send tombstone missing", e)
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = lockAccountPurge(ctx, tx, f.d.AccountID); e != nil {
		t.Fatal(e)
	}
	artifactTouched := false
	e = commitAccountPurgeWithArtifacts(ctx, tx, f.d.AccountID, func(string) error { artifactTouched = true; return nil })
	if !errors.Is(e, ErrAccountPurgeConflict) || artifactTouched {
		t.Fatal("purge erased evidence/cleanup credentials", e, artifactTouched)
	}
	tx.Rollback()
	var exists bool
	if e = db.QueryRow("SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)", f.d.AccountID).Scan(&exists); e != nil || !exists {
		t.Fatal("account lost", e)
	}
}
