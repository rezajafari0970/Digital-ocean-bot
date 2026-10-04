package usercapacity

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestProfileRateBudgetsAreIndependentDurableAndBounded(t *testing.T) {
	db := admissionTestDB(t)
	if _, err := db.Exec(`CREATE TABLE bulk_user_rate_state(panel_id uuid,inbound_id bigint,route_class text NOT NULL DEFAULT '',tokens double precision NOT NULL DEFAULT 0,last_refill_at timestamptz,updated_at timestamptz,PRIMARY KEY(panel_id,inbound_id,route_class))`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Microsecond)
	ctx := context.Background()
	for _, cls := range []string{"DIRECT", "RESIDENTIAL"} {
		n, err := (Service{DB: db}).durableAllowance(ctx, admissionPanel, 1, 10, 100, now, cls)
		if err != nil || n != 0 {
			t.Fatal("new bucket burst", n, err)
		}
	}
	results := make(chan int, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := (Service{DB: db}).durableAllowance(ctx, admissionPanel, 1, 10, 100, now.Add(time.Second), "DIRECT")
			results <- n
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for n := range results {
		total += n
	}
	if total != 10 {
		t.Fatal("concurrent claims exceeded budget", total)
	}
	n, err := (Service{DB: db}).durableAllowance(ctx, admissionPanel, 1, 10, 100, now.Add(time.Second), "RESIDENTIAL")
	if err != nil || n != 10 {
		t.Fatal("other profile consumed budget", n, err)
	}
	n, err = (Service{DB: db}).durableAllowance(ctx, admissionPanel, 1, 10, 100, now.Add(time.Second), "DIRECT")
	if err != nil || n != 0 {
		t.Fatal("service restart reset tokens", n, err)
	}
}
