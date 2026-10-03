package adminapi

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOutputRefreshMutexSerializesRefreshWork(t *testing.T) {
	s := &Server{}
	var active int32
	var peak int32
	run := func() {
		s.OutputRefreshMu.Lock()
		defer s.OutputRefreshMu.Unlock()
		n := atomic.AddInt32(&active, 1)
		if n > atomic.LoadInt32(&peak) {
			atomic.StoreInt32(&peak, n)
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&active, -1)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); run() }()
	}
	wg.Wait()
	if peak != 1 {
		t.Fatalf("peak concurrent refreshes=%d, want 1", peak)
	}
}
