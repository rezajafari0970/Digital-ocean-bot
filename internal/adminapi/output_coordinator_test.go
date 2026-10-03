package adminapi

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
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

func TestPanelRefreshAdmissionIsPerPanel(t *testing.T) {
	s := &Server{OutputPanelRun: map[string]bool{}}
	p1 := readyworker.Panel{ID: "panel-a"}
	p2 := readyworker.Panel{ID: "panel-b"}

	s.OutputPanelMu.Lock()
	s.OutputPanelRun[p1.ID] = true
	s.OutputPanelMu.Unlock()

	s.OutputPanelMu.Lock()
	p1Busy := s.OutputPanelRun[p1.ID]
	p2Busy := s.OutputPanelRun[p2.ID]
	s.OutputPanelMu.Unlock()

	if !p1Busy {
		t.Fatal("panel-a must remain marked busy")
	}
	if p2Busy {
		t.Fatal("panel-b must remain independently schedulable")
	}
}

func TestOutputRefreshConcurrencyIsBounded(t *testing.T) {
	s := &Server{
		OutputPanelRun:   map[string]bool{},
		OutputRefreshSem: make(chan struct{}, 2),
	}
	s.OutputRefreshSem <- struct{}{}
	s.OutputRefreshSem <- struct{}{}
	if s.startPanelOutputRefresh(context.Background(), readyworker.Panel{ID: "panel-c"}) {
		t.Fatal("refresh must be rejected when global capacity is full")
	}
	s.OutputPanelMu.Lock()
	busy := s.OutputPanelRun["panel-c"]
	s.OutputPanelMu.Unlock()
	if busy {
		t.Fatal("rejected panel must not be marked busy")
	}
}

func TestOutputRefreshDefaultCapacity(t *testing.T) {
	s := &Server{OutputPanelRun: map[string]bool{}}
	s.OutputPanelMu.Lock()
	if s.OutputRefreshSem == nil {
		s.OutputRefreshSem = make(chan struct{}, 32)
	}
	capacity := cap(s.OutputRefreshSem)
	s.OutputPanelMu.Unlock()
	if capacity != 32 {
		t.Fatalf("capacity=%d want 32", capacity)
	}
}

func TestRotateOutputPanelsPreventsFixedTailStarvation(t *testing.T) {
	in := []readyworker.Panel{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	got := rotateOutputPanels(in, 2)
	want := []string{"c", "d", "a", "b"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("index %d got=%s want=%s", i, got[i].ID, id)
		}
	}
	if in[0].ID != "a" {
		t.Fatal("rotation must not mutate source ordering")
	}
}
