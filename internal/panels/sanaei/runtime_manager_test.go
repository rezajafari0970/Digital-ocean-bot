package sanaei

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRuntimeManagerRejectsEmptyPanel(t *testing.T) {
	m := &RuntimeManager{}
	if _, e := m.Acquire(context.Background(), ""); e == nil {
		t.Fatal("expected error")
	}
}

func TestRuntimeManagerInvalidateAndPurge(t *testing.T) {
	m := &RuntimeManager{TTL: time.Millisecond, entries: map[string]*runtimeEntry{
		"p": {runtime: &PanelRuntime{PanelID: "p", Session: &PanelSession{}}, opened: time.Now().Add(-time.Second)},
	}}
	m.PurgeExpired()
	if m.Size() != 0 {
		t.Fatalf("size=%d", m.Size())
	}
	m.entries["p"] = &runtimeEntry{runtime: &PanelRuntime{PanelID: "p", Session: &PanelSession{}}, opened: time.Now()}
	m.Invalidate("p")
	if m.Size() != 0 {
		t.Fatalf("size=%d", m.Size())
	}
}

func TestRuntimeManagerConcurrentCacheRead(t *testing.T) {
	r := &PanelRuntime{PanelID: "p", Session: &PanelSession{}}
	m := &RuntimeManager{TTL: time.Minute, entries: map[string]*runtimeEntry{"p": {runtime: r, opened: time.Now()}}}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, e := m.Acquire(context.Background(), "p")
			if e != nil || got != r {
				t.Errorf("got=%p err=%v", got, e)
			}
		}()
	}
	wg.Wait()
}
