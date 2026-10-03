package adminapi

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http/httptest"
	"testing"
)

func TestAccountDeletionPurgesDataAndIsolatesOtherAccount(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	const a = "22222222-2222-4222-8222-222222222222"
	const b = "33333333-3333-4333-8333-333333333333"
	for _, id := range []string{a, b} {
		sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')", id)
		sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", id)
		sqlMust(t, db, "INSERT INTO account_billing_snapshots(account_id,data) VALUES($1,'{\"balance\":\"3.00\"}')", id)
	}
	s := Server{DB: db}
	req := httptest.NewRequest("DELETE", "/api/v1/accounts/"+a, nil)
	req.SetPathValue("id", a)
	req = req.WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Role: auth.Admin}))
	w := httptest.NewRecorder()
	s.deleteAccount(w, req)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM account_deletion_jobs WHERE account_id=$1", a).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	c := app.Container{DB: db}
	if err := c.ProcessAccountDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accounts", "network_profiles", "account_billing_snapshots", "account_deletion_jobs"} {
		col := "account_id"
		if table == "accounts" {
			col = "id"
		}
		if err := db.QueryRow("SELECT count(*) FROM "+table+" WHERE "+col+"=$1", a).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	if err := db.QueryRow("SELECT count(*) FROM account_billing_snapshots WHERE account_id=$1", b).Scan(&n); err != nil || n != 1 {
		t.Fatal("other account damaged", n, err)
	}
}
func TestAccountDeletionRetainsCredentialsWhileResourcesPending(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	const a = "22222222-2222-4222-8222-222222222222"
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,enabled,deletion_requested_at) VALUES($1,'test','digitalocean','test',false,now())", a)
	sqlMust(t, db, "INSERT INTO account_deletion_jobs(account_id) VALUES($1)", a)
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'test-server','RETIRING')", a)
	if err := (app.Container{DB: db}).ProcessAccountDeletions(ctx); err == nil {
		t.Fatal("must remain pending")
	}
	var state string
	if err := db.QueryRow("SELECT runtime_status FROM accounts WHERE id=$1", a).Scan(&state); err != nil || state != "DELETE_PENDING" {
		t.Fatal(state, err)
	}
}
