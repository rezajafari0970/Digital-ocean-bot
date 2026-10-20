package adminapi

import (
	"context"
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
