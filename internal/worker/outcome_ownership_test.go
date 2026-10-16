package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOutcomeReservationFencesLateCompletion(t *testing.T) {
	db := failureLedgerFixture(t)
	s := FailureStore{DB: db}
	ctx := context.Background()
	old, ok, err := s.ReserveOwnedOutcome(ctx, "residential_sync", "panel", "", 90*time.Second)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	var winners int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 18 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, e := s.ReserveOwnedOutcome(ctx, "residential_sync", "panel", "", 90*time.Second)
			if e != nil {
				t.Error(e)
			}
			if ok {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners != 0 {
		t.Fatal("duplicate reservation", winners)
	}
	if _, err = db.Exec("UPDATE worker_item_failures SET next_retry_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	fresh, ok, err := s.ReserveOwnedOutcome(ctx, "residential_sync", "panel", "", 90*time.Second)
	if err != nil || !ok || fresh.Version.Equal(old.Version) {
		t.Fatal(fresh, old, ok, err)
	}
	for _, cause := range []error{nil, errors.New("late failure")} {
		if err = s.RecordOwnedOutcome(ctx, "residential_sync", "panel", "", old, cause); err != nil {
			t.Fatal(err)
		}
		var version time.Time
		var failures int
		if err = db.QueryRow("SELECT next_retry_at,failures FROM worker_item_failures").Scan(&version, &failures); err != nil || !version.Equal(fresh.Version) || failures != 0 {
			t.Fatal("late owner changed replacement", version, failures, err)
		}
	}
	if err = s.RecordOwnedOutcome(ctx, "residential_sync", "panel", "", fresh, errors.New("real failure")); err != nil {
		t.Fatal(err)
	}
	var failures int
	var last string
	if err = db.QueryRow("SELECT failures,last_error FROM worker_item_failures").Scan(&failures, &last); err != nil || failures != 1 || last != "real failure" {
		t.Fatal(failures, last, err)
	}
}
