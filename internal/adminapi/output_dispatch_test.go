package adminapi

import (
	"context"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
)

func TestOutputDispatchDeadlineAndStaleNotification(t *testing.T) {
	for _, stale := range []bool{false, true} {
		s := &Server{OutputRefreshSem: make(chan struct{}, 1)}
		s.OutputRefreshSem <- struct{}{}
		if stale {
			s.OutputRefreshDone = make(chan struct{}, 1)
			s.OutputRefreshDone <- struct{}{}
		}
		panel := readyworker.Panel{ID: "panel"}
		start := time.Now()
		if s.startPanelOutputRefresh(context.Background(), panel) {
			t.Fatal("nonblocking start admitted beyond budget")
		}
		if time.Since(start) > 100*time.Millisecond {
			t.Fatal("direct start blocked")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		after := scheduleOutputPanels([]readyworker.Panel{panel}, "previous", func(p readyworker.Panel) bool { return s.waitPanelOutputRefresh(ctx, p) })
		cancel()
		if after != "previous" || len(s.OutputRefreshSem) != 1 || len(s.OutputPanelRun) != 0 {
			t.Fatal("deadline consumed slot or advanced cursor")
		}
		if time.Since(start) > time.Second {
			t.Fatal("dispatch exceeded deadline")
		}
		if s.OutputRefreshDone == nil {
			t.Fatal("preset semaphore omitted completion notification")
		}
	}
}

func TestOutputDispatchRejectsCanceledAndRunningPanels(t *testing.T) {
	s := &Server{OutputRefreshSem: make(chan struct{}, 1)}
	panel := readyworker.Panel{ID: "panel"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.startPanelOutputRefresh(ctx, panel) || s.waitPanelOutputRefresh(ctx, panel) {
		t.Fatal("canceled parent admitted work")
	}
	if len(s.OutputPanelRun) != 0 || len(s.OutputRefreshSem) != 0 {
		t.Fatal("canceled admission reserved ownership")
	}
	s.OutputPanelRun = map[string]bool{panel.ID: true}
	start := time.Now()
	if s.waitPanelOutputRefresh(context.Background(), panel) {
		t.Fatal("duplicate running panel admitted")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("running panel waited instead of skipping")
	}
}
