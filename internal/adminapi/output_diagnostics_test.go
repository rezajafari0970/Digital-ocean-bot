package adminapi

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRepeatedOutputDiagnosticsPreservesChanges(t *testing.T) {
	var d repeatedOutputDiagnostics
	now := time.Now()
	if !d.shouldLog("panel", "missing_key=3", now) {
		t.Fatal("first diagnostic hidden")
	}
	if d.shouldLog("panel", "missing_key=3", now.Add(time.Second)) {
		t.Fatal("repeated diagnostic not bounded")
	}
	if !d.shouldLog("panel", "missing_key=0", now.Add(2*time.Second)) {
		t.Fatal("recovery hidden")
	}
	if !d.shouldLog("panel", "missing_key=0", now.Add(6*time.Minute)) {
		t.Fatal("periodic status hidden")
	}
	if !d.shouldLog("panel", "missing_key=0", now) {
		t.Fatal("clock regression should not suppress indefinitely")
	}
}
func TestRepeatedOutputDiagnosticsConcurrentBound(t *testing.T) {
	var d repeatedOutputDiagnostics
	var wg sync.WaitGroup
	for i := 0; i < 1100; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); d.shouldLog(fmt.Sprint(i), "ok", time.Now()) }(i)
	}
	wg.Wait()
	if len(d.entries) > 1024 {
		t.Fatalf("unbounded entries: %d", len(d.entries))
	}
}
