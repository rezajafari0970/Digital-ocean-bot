package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const ipa = "11111111-1111-4111-8111-111111111111"
const ipb = "22222222-2222-4222-8222-222222222222"

func ipSeed(t *testing.T, db *sql.DB) {
	t.Helper()
	sqlMust(t, db, `INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'fixture <account>','digitalocean','test'),($2,'second','vultr','test')`, ipa, ipb)
}
func ipObs(limit *int) providers.IPObservation {
	u := 0
	return providers.IPObservation{Complete: true, ObservedAt: time.Now().UTC(), Resources: []providers.IPResource{}, Quotas: []providers.IPQuota{{Family: 4, Kind: "reserved", Limit: limit, Used: &u, Scope: "account", Source: "fixture", ObservedAt: time.Now().UTC()}}}
}
func ipAllow(t *testing.T, db *sql.DB, a string) {
	sqlMust(t, db, "UPDATE ip_inventory_lanes SET next_attempt_at=NULL,lease_until=NULL,rate_limit_until=NULL WHERE account_id=$1", a)
}
func TestIPInventoryLastGoodUnknownAndRateLimit(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s := ipam.InventoryStore{DB: db}
	n := 10
	o := ipObs(&n)
	tick, e := s.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, tick, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, ipa)
	tick, _ = s.Begin(ctx, ipa)
	o = ipObs(nil)
	if e = s.Finish(ctx, tick, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	var current sql.NullInt64
	var last int
	if e = db.QueryRow("SELECT current_limit,last_known_limit FROM ip_quota_evidence WHERE account_id=$1", ipa).Scan(&current, &last); e != nil || current.Valid || last != 10 {
		t.Fatal(current, last, e)
	}
	ipAllow(t, db, ipa)
	tick, _ = s.Begin(ctx, ipa)
	if e = s.Finish(ctx, tick, nil, "rate_limited", 429, 2*time.Hour); e != nil {
		t.Fatal(e)
	}
	var klass string
	var delay float64
	var good bool
	e = db.QueryRow("SELECT error_class,EXTRACT(epoch FROM next_attempt_at-clock_timestamp()),last_good IS NOT NULL FROM ip_inventory_lanes WHERE account_id=$1", ipa).Scan(&klass, &delay, &good)
	if e != nil || klass != "rate_limited" || delay < 7190 || !good {
		t.Fatal(klass, delay, good, e)
	}
	w := httptest.NewRecorder()
	(&Server{DB: db}).ipInventory(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `"limit":null`) || !strings.Contains(body, `"last_known_limit":10`) || !strings.Contains(body, `"allocation_enabled":false`) {
		t.Fatal(w.Code, body)
	}
}
func TestIPInventoryIndependentLanesRestartAndLateResponse(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	s := ipam.InventoryStore{DB: db}
	ctx := context.Background()
	first, e := s.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Begin(ctx, ipa); !errors.Is(e, ipam.ErrLaneBusy) {
		t.Fatal("duplicate reader", e)
	}
	other, e := s.Begin(ctx, ipb)
	if e != nil {
		t.Fatal("one account blocked another", e)
	}
	if e = s.Finish(ctx, other, nil, "network", 0, 0); e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, ipa)
	restart := ipam.InventoryStore{DB: db}
	second, e := restart.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	n := 4
	o := ipObs(&n)
	if e = restart.Finish(ctx, second, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	n = 99
	o = ipObs(&n)
	if e = s.Finish(ctx, first, &o, "", 0, 0); !errors.Is(e, ipam.ErrSuperseded) {
		t.Fatal("late response overwrote current", e)
	}
	if e = restart.Finish(ctx, second, &o, "", 0, 0); !errors.Is(e, ipam.ErrReceiptConflict) {
		t.Fatal("conflicting finish", e)
	}
	var limit, count int
	if e := db.QueryRow("SELECT current_limit FROM ip_quota_evidence WHERE account_id=$1", ipa).Scan(&limit); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow("SELECT count(*) FROM ip_inventory_events WHERE account_id=$1", ipa).Scan(&count); e != nil {
		t.Fatal(e)
	}
	if limit != 4 || count != 2 {
		t.Fatal(limit, count)
	}
}
func TestIPInventoryConcurrentAdmissionAndCapacityChange(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	s := ipam.InventoryStore{DB: db}
	ctx := context.Background()
	var wg sync.WaitGroup
	ch := make(chan ipam.ReadTicket, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticket, e := s.Begin(ctx, ipa)
			if e == nil {
				ch <- ticket
			} else if !errors.Is(e, ipam.ErrLaneBusy) {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(ch)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if len(ch) != 1 {
		t.Fatal("parallel admission", len(ch))
	}
	tick := <-ch
	n := 10
	o := ipObs(&n)
	if e := s.Finish(ctx, tick, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, ipa)
	tick, _ = s.Begin(ctx, ipa)
	n = 3
	o = ipObs(&n)
	if e := s.Finish(ctx, tick, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	var changed bool
	if e := db.QueryRow("SELECT capacity_changed FROM ip_quota_evidence WHERE account_id=$1", ipa).Scan(&changed); e != nil {
		t.Fatal(e)
	}
	if !changed {
		t.Fatal("capacity shrink hidden")
	}
}
func TestIPInventoryFailureToCommitDoesNotClaimSuccess(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	s := ipam.InventoryStore{DB: db}
	ctx := context.Background()
	tick, _ := s.Begin(ctx, ipa)
	o := ipObs(nil)
	sqlMust(t, db, `CREATE FUNCTION reject_ip_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture unavailable'; END $$; CREATE TRIGGER reject_ip_event BEFORE INSERT ON ip_inventory_events FOR EACH ROW EXECUTE FUNCTION reject_ip_event()`)
	if e := s.Finish(ctx, tick, &o, "", 0, 0); e == nil {
		t.Fatal("receipt failure ignored")
	}
	var state string
	var good bool
	if e := db.QueryRow("SELECT state,last_good IS NOT NULL FROM ip_inventory_lanes WHERE account_id=$1", ipa).Scan(&state, &good); e != nil {
		t.Fatal(e)
	}
	if state != "READING" || good {
		t.Fatal(state, good)
	}
}

func TestIPInventoryDuplicateReceiptsPreserveDurableOutcome(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s := ipam.InventoryStore{DB: db}
	tick, e := s.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, tick, nil, "transport", 0, 0); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(ctx, tick, nil, "transport", 0, 0); e != nil {
		t.Fatal("identical failure", e)
	}
	o := ipObs(nil)
	if e = s.Finish(ctx, tick, &o, "", 0, 0); !errors.Is(e, ipam.ErrReceiptConflict) {
		t.Fatal("failure changed to success", e)
	}
	ipAllow(t, db, ipa)
	old, e := s.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, ipa)
	newer, e := s.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = s.Finish(ctx, old, &o, "", 0, 0); !errors.Is(e, ipam.ErrSuperseded) {
			t.Fatal("superseded changed on repeat", e)
		}
	}
	o = ipObs(nil)
	for i := 0; i < 2; i++ {
		if e = s.Finish(ctx, newer, &o, "", 0, 0); e != nil {
			t.Fatal("identical success", e)
		}
	}
	changed := ipObs(nil)
	if e = s.Finish(ctx, newer, &changed, "", 0, 0); !errors.Is(e, ipam.ErrReceiptConflict) {
		t.Fatal("conflicting payload accepted", e)
	}
}
func TestIPInventorySharedServiceAdmissionAndRetryAfter(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s := ipam.InventoryStore{DB: db}
	tick, e := s.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	read := func(context.Context, string) (providers.IPObservation, error) { calls++; return ipObs(nil), nil }
	if _, e = s.Refresh(ctx, ipa, read); !errors.Is(e, ipam.ErrLaneBusy) || calls != 0 {
		t.Fatal("service bypassed panel ticket", e, calls)
	}
	if _, e = s.Refresh(ctx, ipb, read); e != nil || calls != 1 {
		t.Fatal("account lanes coupled", e, calls)
	}
	if e = s.Finish(ctx, tick, nil, "transport", 0, 0); e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, ipa)
	rate := func(context.Context, string) (providers.IPObservation, error) {
		calls++
		return providers.IPObservation{}, &providers.Error{Class: providers.ErrorRateLimited, StatusCode: 429, RetryAfter: time.Hour}
	}
	result, e := s.Refresh(ctx, ipa, rate)
	if e != nil || result.State != "FAILED" || result.ErrorClass != "rate_limited" {
		t.Fatal(result, e)
	}
	for i := 0; i < 2; i++ {
		if _, e = s.Refresh(ctx, ipa, read); !errors.Is(e, ipam.ErrLaneBusy) || calls != 2 {
			t.Fatal("cooldown bypass", e, calls)
		}
	}
	ipAllow(t, db, ipa)
	started := make(chan struct{})
	done := make(chan struct{})
	finished := make(chan error)
	go func() {
		_, e := s.Refresh(ctx, ipa, func(context.Context, string) (providers.IPObservation, error) {
			close(started)
			<-done
			return ipObs(nil), nil
		})
		finished <- e
	}()
	<-started
	if _, e = s.Refresh(ctx, ipa, read); !errors.Is(e, ipam.ErrLaneBusy) {
		t.Fatal("concurrent reader bypass", e)
	}
	close(done)
	if e = <-finished; e != nil {
		t.Fatal(e)
	}
}

func TestIPInventoryLateRateLimitSurvivesTakeoverAndSuccess(t *testing.T) {
	for _, lateFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "late-429-before-new-success", false: "late-429-after-new-success"}[lateFirst], func(t *testing.T) {
			db := adminTestDB(t)
			ipSeed(t, db)
			ctx := context.Background()
			s := ipam.InventoryStore{DB: db}
			old, e := s.Begin(ctx, ipa)
			if e != nil {
				t.Fatal(e)
			}
			ipAllow(t, db, ipa)
			fresh, e := s.Begin(ctx, ipa)
			if e != nil {
				t.Fatal(e)
			}
			n := 7
			o := ipObs(&n)
			late := func() {
				t.Helper()
				if e := s.Finish(ctx, old, nil, "rate_limited", 429, time.Hour); !errors.Is(e, ipam.ErrSuperseded) {
					t.Fatal(e)
				}
			}
			success := func() {
				t.Helper()
				if e := s.Finish(ctx, fresh, &o, "", 0, 0); e != nil {
					t.Fatal(e)
				}
			}
			if lateFirst {
				late()
				success()
			} else {
				success()
				late()
			}
			var deadline time.Time
			var quota int
			if e := db.QueryRow("SELECT rate_limit_until FROM ip_inventory_lanes WHERE account_id=$1", ipa).Scan(&deadline); e != nil {
				t.Fatal(e)
			}
			if time.Until(deadline) < 59*time.Minute {
				t.Fatal("cooldown lost", deadline)
			}
			if e := db.QueryRow("SELECT current_limit FROM ip_quota_evidence WHERE account_id=$1", ipa).Scan(&quota); e != nil || quota != 7 {
				t.Fatal("new evidence overwritten", quota, e)
			}
			late()
			var again time.Time
			if e := db.QueryRow("SELECT rate_limit_until FROM ip_inventory_lanes WHERE account_id=$1", ipa).Scan(&again); e != nil || !again.Equal(deadline) {
				t.Fatal("duplicate extended restriction", e, again, deadline)
			}
			if _, e = s.Begin(ctx, ipa); !errors.Is(e, ipam.ErrLaneBusy) {
				t.Fatal("late 429 bypass", e)
			}
		})
	}
}
