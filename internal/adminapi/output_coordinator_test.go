package adminapi

import (
	"os"
	"strings"
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

func TestOutputHandlersDoNotSynchronouslyRefreshFleet(t *testing.T) {
	outputSource, err := os.ReadFile("output.go")
	if err != nil {
		t.Fatal(err)
	}
	snapshotSource, err := os.ReadFile("output_snapshot.go")
	if err != nil {
		t.Fatal(err)
	}
	outputHandler := sourceFunction(string(outputSource), "func (s *Server) outputConfigs", "func (s *Server) refreshOutputLive")
	if strings.Contains(outputHandler, "refreshOutputLive") {
		t.Fatal("output handler must not block on a full-fleet refresh")
	}
	shareHandler := sourceFunction(string(snapshotSource), "func (s *Server) sharedOutput", "func (s *Server) outputSnapshotResponse")
	if strings.Contains(shareHandler, "refreshOutputLive") {
		t.Fatal("share handler must not block on a full-fleet refresh")
	}
}

func sourceFunction(src, start, next string) string {
	i := strings.Index(src, start)
	if i < 0 {
		return ""
	}
	j := strings.Index(src[i:], next)
	if j < 0 {
		return src[i:]
	}
	return src[i : i+j]
}
