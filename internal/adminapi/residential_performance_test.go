package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func perfUUID() string { id, _ := sanaei.UUIDv4(); return id }
func TestPerformanceDurableDeadlineConflictAndRecovery(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	store := residentialperf.Store{DB: db}
	_, _, panel := seedPanel(t, db, "http://test.invalid")
	proxy := perfUUID()
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'ads1','socks5','localhost',1080,'healthy',now(),'residential-ads-test')", proxy)
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	var rev int64
	db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev)
	sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,plan_hash,state,pool_enabled,verified_at) VALUES($1,$2,'before','APPLIED',true,now())", panel, rev)
	config := residentialperf.Balanced()
	q := residentialperf.Request{RequestID: perfUUID(), Action: "start", PanelIDs: []string{panel}, Config: &config, Minutes: 15, BaseRevision: rev, BasePlan: "before"}
	first, e := store.Do(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := store.Do(ctx, q)
	if e != nil || replay != first {
		t.Fatal("lost-response replay", e)
	}
	changed := q
	changed.Minutes = 20
	if _, e = store.Do(ctx, changed); e == nil {
		t.Fatal("request identity not enforced")
	}
	keep := residentialperf.Request{RequestID: perfUUID(), ExperimentID: first.ExperimentID, Action: "keep", ExpectedVersion: first.Version}
	if _, e = store.Do(ctx, keep); e == nil {
		t.Fatal("kept unverified")
	}
	original := map[string]any{"routing": map[string]any{"domainStrategy": "IPIfNonMatch"}}
	f := residentialperf.Capture(original)
	a, e := store.Prepare(ctx, panel, 1, f)
	if e != nil || a.Baseline == nil {
		t.Fatal(e)
	}
	tuned := map[string]any{"routing": map[string]any{"domainStrategy": "IPIfNonMatch"}}
	residentialperf.Apply(tuned, &config)
	if e = store.Plan(ctx, panel, 1, residentialperf.Capture(tuned)); e != nil {
		t.Fatal(e)
	}
	// A process restart after remote mutation but before its response can reconcile the durable pending values.
	store = residentialperf.Store{DB: db}
	if _, e = store.Prepare(ctx, panel, 1, residentialperf.Capture(tuned)); e != nil {
		t.Fatal("lost apply", e)
	}
	external := map[string]any{"routing": map[string]any{"domainStrategy": "IPOnDemand"}}
	if _, e = store.Prepare(ctx, panel, 1, residentialperf.Capture(external)); e == nil {
		t.Fatal("external write overwritten")
	}
	if e = store.Applied(ctx, panel, 1); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "UPDATE panel_routing_state SET state='APPLIED',performance_generation=1,verified_at=now() WHERE panel_id=$1", panel)
	// Deadline races with keep: serialization ensures an expired experiment cannot be kept.
	sqlMust(t, db, "UPDATE residential_performance_experiments SET deadline=now()-interval '1 second' WHERE id=$1", first.ExperimentID)
	var wg sync.WaitGroup
	wg.Add(2)
	var te, ke error
	go func() { defer wg.Done(); te = store.Tick(ctx) }()
	go func() { defer wg.Done(); _, ke = store.Do(ctx, keep) }()
	wg.Wait()
	if te != nil || ke == nil {
		t.Fatal("deadline/keep race", te, ke)
	}
	a, e = store.Load(ctx, panel)
	if e != nil || a.Generation != 2 || a.Config != nil {
		t.Fatal("rollback desired state", e)
	}
	if e = store.Applied(ctx, panel, 1); e == nil {
		t.Fatal("stale apply receipt accepted")
	}
	// Add an endpoint while rollback is pending: the durable operation must preserve it.
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag) VALUES($1,'Ads2','socks5','localhost',1081,'residential-ads-second')", perfUUID())
	if e = store.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	var state string
	db.QueryRow("SELECT state FROM residential_performance_experiments WHERE id=$1", first.ExperimentID).Scan(&state)
	if state != "ROLLING_BACK" {
		t.Fatal("rollback prematurely complete", state)
	}
	residentialperf.Restore(tuned, *a.Baseline)
	if e = store.Plan(ctx, panel, 2, residentialperf.Capture(tuned)); e != nil {
		t.Fatal(e)
	}
	if e = store.Applied(ctx, panel, 2); e != nil {
		t.Fatal(e)
	}
	if e = store.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	db.QueryRow("SELECT state FROM residential_performance_experiments WHERE id=$1", first.ExperimentID).Scan(&state)
	if state != "ROLLED_BACK" {
		t.Fatal(state)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM residential_proxies").Scan(&n)
	if n != 2 {
		t.Fatal("rollback deleted proxies")
	}
	a, _ = store.Load(ctx, panel)
	if a.Baseline != nil {
		t.Fatal("rolled back fields still owned")
	}
	// New test; three verification failures revert it without claiming server restoration.
	db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev)
	sqlMust(t, db, "UPDATE panel_routing_state SET revision=$2,state='APPLIED',verified_at=now() WHERE panel_id=$1", panel, rev)
	q.RequestID = perfUUID()
	q.BaseRevision = rev
	next, e := store.Do(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		store.Failed(ctx, panel, 3, errors.New("network failed"))
	}
	db.QueryRow("SELECT state FROM residential_performance_experiments WHERE id=$1", next.ExperimentID).Scan(&state)
	if state != "ROLLING_BACK" {
		t.Fatal("no automatic rollback", state)
	}
}
func TestPerformanceAPIAuthAndStatus(t *testing.T) {
	db := adminTestDB(t)
	s := Server{DB: db}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	s.residentialPerformanceStatus(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	w = httptest.NewRecorder()
	s.residentialPerformanceStatus(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out["defaults"] == nil {
		t.Fatal("missing defaults")
	}
	r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"action":"start","unknown":true}`)).WithContext(r.Context())
	w = httptest.NewRecorder()
	s.residentialPerformanceAction(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestPerformancePromotionKeepAndRetirement(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	store := residentialperf.Store{DB: db}
	_, _, one := seedPanel(t, db, "http://one.invalid")
	_, droplet, two := seedPanel(t, db, "http://two.invalid")
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1,'ads1','socks5','localhost',1080,'healthy',now(),'residential-ads-one')", perfUUID())
	sqlMust(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	var rev int64
	db.QueryRow("SELECT revision FROM residential_routing_control").Scan(&rev)
	for _, p := range []string{one, two} {
		sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,revision,plan_hash,state,pool_enabled,verified_at) VALUES($1,$2,'baseline','APPLIED',true,now())", p, rev)
	}
	c := residentialperf.Balanced()
	q := residentialperf.Request{RequestID: perfUUID(), Action: "start", PanelIDs: []string{one}, Config: &c, Minutes: 5, BaseRevision: rev, BasePlan: "baseline"}
	result, e := store.Do(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	verify := func(panel string, gen int64) {
		t.Helper()
		if e := store.Applied(ctx, panel, gen); e != nil {
			t.Fatal(e)
		}
		sqlMust(t, db, "UPDATE panel_routing_state SET state='APPLIED',performance_generation=$2,verified_at=now() WHERE panel_id=$1", panel, gen)
	}
	verify(one, 1)
	promote := residentialperf.Request{RequestID: perfUUID(), ExperimentID: result.ExperimentID, ExpectedVersion: result.Version, Action: "promote", PanelIDs: []string{two}}
	result, e = store.Do(ctx, promote)
	if e != nil {
		t.Fatal(e)
	}
	keep := residentialperf.Request{RequestID: perfUUID(), ExperimentID: result.ExperimentID, ExpectedVersion: result.Version, Action: "keep"}
	if _, e = store.Do(ctx, keep); e == nil {
		t.Fatal("kept before promoted server verification")
	}
	verify(two, 1)
	result, e = store.Do(ctx, keep)
	if e != nil || result.State != "KEPT" {
		t.Fatal(e, result)
	}
	q.RequestID = perfUUID()
	if _, e = store.Do(ctx, q); e == nil {
		t.Fatal("new experiment hid existing rollback")
	}
	// Keeping stops deadline rollback, but explicit rollback still works.
	sqlMust(t, db, "UPDATE residential_performance_experiments SET deadline=now()-interval '1 minute'")
	if e = store.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	var state string
	db.QueryRow("SELECT state FROM residential_performance_experiments").Scan(&state)
	if state != "KEPT" {
		t.Fatal(state)
	}
	rollback := residentialperf.Request{RequestID: perfUUID(), ExperimentID: result.ExperimentID, ExpectedVersion: result.Version, Action: "rollback"}
	result, e = store.Do(ctx, rollback)
	if e != nil {
		t.Fatal(e)
	}
	// A deleted server is retired explicitly; an unavailable living server cannot be called restored.
	sqlMust(t, db, "UPDATE droplets SET state='DELETED' WHERE id=$1", droplet)
	verify(one, 2)
	if e = store.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	db.QueryRow("SELECT state FROM residential_performance_experiments").Scan(&state)
	if state != "ROLLED_BACK" {
		t.Fatal(state)
	}
	db.QueryRow("SELECT state FROM residential_performance_targets WHERE panel_id=$1", two).Scan(&state)
	if state != "RETIRED" {
		t.Fatal(state)
	}
}
