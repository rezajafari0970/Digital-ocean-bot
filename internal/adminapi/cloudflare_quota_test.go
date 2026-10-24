package adminapi

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"testing"
)

type cfQuotaFixture struct {
	cfDriverFixture
	quota cloudflare.DNSQuota
	reads int
}

func (d *cfQuotaFixture) DNSQuota(context.Context, string) (cloudflare.DNSQuota, error) {
	d.reads++
	return d.quota, nil
}
func TestCloudflareDNSQuotaAdmissionRecoveryAndCleanup(t *testing.T) {
	db := adminTestDB(t)
	cfSeed(t, db)
	ctx := context.Background()
	limit := 1
	d := &cfQuotaFixture{cfDriverFixture: cfDriverFixture{records: map[string]cloudflare.Record{}}, quota: cloudflare.DNSQuota{Limit: &limit, Usage: 1}}
	svc := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	run := func() {
		t.Helper()
		sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
		sqlMust(t, db, "UPDATE cloudflare_records SET next_attempt_at=now()")
		if e := svc.RunOne(ctx); e != nil {
			t.Fatal(e)
		}
	}
	schedule := func() { sqlMust(t, db, "UPDATE cloudflare_schedules SET next_batch_at=now()") }
	refresh := func() { sqlMust(t, db, "UPDATE cloudflare_zones SET dns_quota_next_attempt_at=now()") }
	run()
	var n int
	db.QueryRow("SELECT count(*) FROM cloudflare_records").Scan(&n)
	if n != 0 || d.creates != 0 {
		t.Fatal("full zone created intents", n, d.creates)
	}
	d.quota.Usage = 0
	refresh()
	schedule()
	run()
	if d.creates != 1 {
		t.Fatal("capacity recovery did not resume", d.creates)
	}
	// A newly confirmed record consumes capacity immediately, before another API usage refresh.
	schedule()
	run()
	db.QueryRow("SELECT count(*) FROM cloudflare_records").Scan(&n)
	if n != 1 || d.creates != 1 || d.reads != 2 {
		t.Fatal("cached quota overbooked", n, d.creates, d.reads)
	}
	// Even when full, expiry cleanup must continue and ownership must be checked.
	d.quota.Usage = 1
	refresh()
	sqlMust(t, db, "UPDATE cloudflare_records SET expires_at=now()-interval '1 minute'")
	run()
	run()
	db.QueryRow("SELECT count(*) FROM cloudflare_records WHERE state='DELETED'").Scan(&n)
	if n != 1 || d.deletes != 1 {
		t.Fatal("quota blocked cleanup", n, d.deletes)
	}
	d.quota.Usage = 0
	refresh()
	schedule()
	run()
	if d.creates != 2 {
		t.Fatal("released capacity not reusable", d.creates)
	}
}

func TestCloudflarePartialQuotaBatchPreservesInterval(t *testing.T) {
	db := adminTestDB(t)
	cfSeed(t, db)
	ctx := context.Background()
	limit := 1
	sqlMust(t, db, "UPDATE config_driver_profiles SET spec=jsonb_set(jsonb_set(spec,'{cloudflare_records}','2'::jsonb),'{cloudflare_max_active}','6'::jsonb)")
	d := &cfQuotaFixture{cfDriverFixture: cfDriverFixture{records: map[string]cloudflare.Record{}}, quota: cloudflare.DNSQuota{Limit: &limit}}
	svc := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	if e := svc.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	var preserves bool
	if e := db.QueryRow("SELECT next_batch_at>now()+interval '29 minutes' FROM cloudflare_schedules").Scan(&preserves); e != nil || !preserves || d.creates != 1 {
		t.Fatal("partial batch lost interval", e, preserves, d.creates)
	}
	// More quota arrives, but a completed partial batch must not refill early.
	limit = 8
	sqlMust(t, db, "UPDATE cloudflare_zones SET dns_quota_next_attempt_at=now()")
	sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
	if e := svc.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM cloudflare_records").Scan(&n)
	if n != 1 || d.creates != 1 {
		t.Fatal("partial batch repeated inside interval", n, d.creates)
	}
}

// Under a real provider quota, renewing an already covered server must not
// consume the last slot before a new server gets its first endpoint.
func TestCloudflareQuotaPrioritizesUncoveredServers(t *testing.T) {
	db := adminTestDB(t)
	account, profile, covered := cfSeed(t, db)
	ctx := context.Background()
	limit := 2
	driver := &cfQuotaFixture{cfDriverFixture: cfDriverFixture{records: map[string]cloudflare.Record{}}, quota: cloudflare.DNSQuota{Limit: &limit}}
	svc := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return driver, nil }}
	if err := svc.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	_, drop, uncovered := seedPanel(t, db, "http://new-panel.invalid")
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.6' WHERE droplet_id=$1", drop)
	sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec) SELECT profile_id,$2,managed_key,'fixture',applied_spec FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$3", profile, uncovered, covered)
	sqlMust(t, db, "UPDATE cloudflare_schedules SET next_batch_at=now()-interval '1 hour' WHERE panel_id=$1", covered)
	sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
	if err := svc.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	var coveredCount, newCount int
	if err := db.QueryRow("SELECT count(*) FILTER(WHERE panel_id=$1),count(*) FILTER(WHERE panel_id=$2) FROM cloudflare_records WHERE state<>'DELETED'", covered, uncovered).Scan(&coveredCount, &newCount); err != nil {
		t.Fatal(err)
	}
	if coveredCount != 1 || newCount != 1 || driver.creates != 2 {
		t.Fatal("quota did not cover both servers", coveredCount, newCount, driver.creates)
	}
	// A scheduler pass cannot exceed the same authoritative cached quota.
	sqlMust(t, db, "UPDATE cloudflare_schedules SET next_batch_at=now()")
	sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
	if err := svc.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	if driver.creates != 2 {
		t.Fatal("quota overbooked", driver.creates)
	}
	missing := perfUUID()
	if _, err := svc.ProbeEndpoint(ctx, account, profile, missing); !errors.Is(err, cloudflare.ErrDNSQuotaUnavailable) {
		t.Fatal("full quota disguised as propagation", err)
	}
	sqlMust(t, db, "UPDATE cloudflare_zones SET dns_record_quota=NULL")
	if _, err := svc.ProbeEndpoint(ctx, account, profile, missing); errors.Is(err, cloudflare.ErrDNSQuotaUnavailable) {
		t.Fatal("unknown quota claimed exhausted")
	}
}
