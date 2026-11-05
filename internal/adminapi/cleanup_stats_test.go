package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"testing"
)

func TestCleanupAccountsExcludedFromEveryActiveStatistic(t *testing.T) {
	db := adminTestDB(t)
	active, _, _ := seedPanel(t, db, "http://active.invalid")
	cleanup, d, _ := seedPanel(t, db, "http://cleanup.invalid")
	sqlMust(t, db, "UPDATE accounts SET name='cleanup-fixture',enabled=false,runtime_status='DELETE_PENDING',deletion_requested_at=now() WHERE id=$1", cleanup)
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", d)
	var raw []byte
	if e := db.QueryRow(dashboardCountsSQL).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var v struct {
		TotalAccounts   int                   `json:"account_health_total"`
		Servers         int                   `json:"total_servers"`
		PendingAccounts int                   `json:"pending_deletion_accounts"`
		PendingServers  int                   `json:"pending_deletion_servers"`
		Accounts        []struct{ ID string } `json:"account_health"`
		Details         []struct {
			AccountName string `json:"account_name"`
		} `json:"server_health"`
	}
	if e := json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	if v.TotalAccounts != 1 || v.Servers != 1 || v.PendingAccounts != 0 || v.PendingServers != 0 || len(v.Accounts) != 1 || v.Accounts[0].ID != active {
		t.Fatal(string(raw))
	}
	for _, x := range v.Details {
		if x.AccountName == "cleanup-fixture" {
			t.Fatal("cleanup server in active diagnostics")
		}
	}
	var kept int
	db.QueryRow("SELECT count(*) FROM droplets WHERE account_id=$1", cleanup).Scan(&kept)
	if kept != 1 {
		t.Fatal("statistics erased cleanup evidence")
	}
}
func TestExplicitLocalPurgeSupportsSettledCleanupWithoutProviderRestriction(t *testing.T) {
	db := adminTestDB(t)
	a, _, _ := seedPurgeDependencies(t, db)
	sqlMust(t, db, "UPDATE accounts SET provider_state='ACTIVE' WHERE id=$1", a)
	confirm := purgeConfirmation(t, db, a)
	if e := app.QueueSavedAccountPurge(context.Background(), db, a, "ACTIVE", 1, confirm); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", a)
	if e := app.PurgeSavedAccount(context.Background(), db, a, "ACTIVE", 1, confirm); e == nil {
		t.Fatal("active account purged")
	}
	sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", a)
	if e := app.PurgeSavedAccount(context.Background(), db, a, "ACTIVE", 1, confirm); e != nil {
		t.Fatal(e)
	}
	var kept int
	db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&kept)
	if kept != 0 {
		t.Fatal("settled local purge incomplete")
	}
}
