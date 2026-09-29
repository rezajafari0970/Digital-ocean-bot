package sanaei

import (
	"context"
	"errors"
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

func TestCircuitDelay(t *testing.T) {
	if circuitDelay(1) != 0 || circuitDelay(2) != 0 {
		t.Fatal("early failures must retry normally")
	}
	if circuitDelay(3) != 30*time.Second {
		t.Fatal("third failure cooldown")
	}
	if circuitDelay(5) != 2*time.Minute {
		t.Fatal("fifth failure cooldown")
	}
}
func TestRuntimeManagerCircuitOpen(t *testing.T) {
	m := &RuntimeManager{circuits: map[string]circuitState{"p": {failures: 3, until: time.Now().Add(time.Minute)}}}
	_, err := m.Acquire(context.Background(), "p")
	if !errors.Is(err, ErrRuntimeCircuitOpen) {
		t.Fatalf("err=%v", err)
	}
}
