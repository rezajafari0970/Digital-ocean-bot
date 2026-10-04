package adminapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

func TestIndependentProfilesMigrationAndAPI(t *testing.T) {
	db := adminTestDBThrough(t, 147)
	sqlMust(t, db, `INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second,generate_direct,generate_residential,revision) VALUES('reality',true,'[443,1212]',2,100,true,true,29) ON CONFLICT(policy_key) DO UPDATE SET enabled=true,ports='[443,1212]',target_users_per_inbound=2,users_per_second=100,generate_direct=true,generate_residential=true,revision=29`)
	raw, err := os.ReadFile("../../migrations/000148_independent_reality_profiles.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, string(raw))
	intervalDDL, err := os.ReadFile("../../migrations/000149_profile_creation_interval.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlMust(t, db, string(intervalDDL))
	var n, total, revision int
	if err = db.QueryRow(`SELECT count(*),sum(target_users_per_inbound) FROM reality_config_profiles`).Scan(&n, &total); err != nil || n != 2 || total != 2 {
		t.Fatal(n, total, err)
	}
	db.QueryRow(`SELECT revision FROM global_config_policies WHERE policy_key='reality'`).Scan(&revision)
	if revision != 29 {
		t.Fatal("migration changed live policy")
	}
	s := Server{DB: db}
	interval := 360
	body := globalConfigRequest{CreationIntervalSeconds: &interval, RouteClass: "RESIDENTIAL", ProfileRevision: 1, Enabled: true, Ports: []int{443}, TargetUsersPerInbound: 1, UserQuotaExpression: "100", UserLifetimeExpression: "30", DeviceLimit: 2, UsersPerSecond: 1, SNISelectionMode: "scored"}
	save := func(x globalConfigRequest) int {
		b, _ := json.Marshal(x)
		w := httptest.NewRecorder()
		s.putGlobalConfig(w, httptest.NewRequest("PUT", "/api/v1/configs", bytes.NewReader(b)))
		return w.Code
	}
	if code := save(body); code != 200 {
		t.Fatal("target 1 must work per class", code)
	}
	var q int64
	var ports string
	var rate int
	if err = db.QueryRow(`SELECT user_quota_bytes,ports::text,users_per_second FROM reality_config_profiles WHERE route_class='DIRECT'`).Scan(&q, &ports, &rate); err != nil || q != 0 || ports != "[443, 1212]" || rate != 100 {
		t.Fatal("other profile changed", q, ports, rate, err)
	}
	if code := save(body); code != 409 {
		t.Fatal("stale form accepted", code)
	}
	body.CreationIntervalSeconds = nil // Older API clients preserve the saved interval.
	body.ProfileRevision = 2
	body.SNISelectionMode = "manual"
	body.ManualSNIs = []string{"example.com"}
	if code := save(body); code != 409 {
		t.Fatal("profile overwrote shared SNI", code)
	}
	body.SNISelectionMode = "scored"
	body.ManualSNIs = nil
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- save(body) }()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for code := range results {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("concurrent edits lost update", counts)
	}
	var savedInterval int
	if err = db.QueryRow(`SELECT creation_interval_seconds FROM reality_config_profiles WHERE route_class='RESIDENTIAL'`).Scan(&savedInterval); err != nil || savedInterval != 360 {
		t.Fatal(savedInterval, err)
	}
	// Disabling one profile while the shared master is paused cannot rearm it.
	sqlMust(t, db, `UPDATE global_config_policies SET enabled=false WHERE policy_key='reality'`)
	body.ProfileRevision = 3
	body.Enabled = false
	if code := save(body); code != 200 {
		t.Fatal(code)
	}
	var master bool
	db.QueryRow(`SELECT enabled FROM global_config_policies WHERE policy_key='reality'`).Scan(&master)
	if master {
		t.Fatal("disabled edit rearmed master")
	}
	body.ProfileRevision = 4
	body.UsersPerSecond = 10000
	if code := save(body); code != 400 {
		t.Fatal("creation rate confused with concurrent users", code)
	}
}
