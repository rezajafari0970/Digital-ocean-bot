package readyworker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fake struct {
	mu          sync.Mutex
	active, max int
	fail        string
}

func (f *fake) Eligible(context.Context) ([]Panel, error) {
	return []Panel{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}, {"f"}, {"g"}}, nil
}
func (f *fake) Bootstrap(_ context.Context, p Panel) error {
	f.mu.Lock()
	f.active++
	if f.active > f.max {
		f.max = f.active
	}
	f.mu.Unlock()
	time.Sleep(time.Millisecond)
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	if p.ID == f.fail {
		return errors.New("fail")
	}
	return nil
}
func (f *fake) Reconcile(context.Context, Panel, bool) error { return nil }
func TestWorkerBoundedDryRun(t *testing.T) {
	f := &fake{}
	rs, e := (Worker{Adapter: f, Concurrency: 2, DryRun: true}).Run(context.Background())
	if e != nil || len(rs) != 7 || f.max > 2 {
		t.Fatalf("%v %d %d", e, len(rs), f.max)
	}
	for _, r := range rs {
		if !r.Bootstrapped || !r.Reconciled || !r.DryRun || r.Err != nil {
			t.Fatalf("%+v", r)
		}
	}
}
func TestBootstrapFailureStopsPanelOnly(t *testing.T) {
	f := &fake{fail: "c"}
	rs, e := (Worker{Adapter: f, Concurrency: 3, DryRun: true}).Run(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	failed := 0
	ok := 0
	for _, r := range rs {
		if r.Err != nil {
			failed++
		} else if r.Reconciled {
			ok++
		}
	}
	if failed != 1 || ok != 6 {
		t.Fatalf("failed=%d ok=%d", failed, ok)
	}
}
