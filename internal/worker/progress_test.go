package worker

import (
	"errors"
	"sync"
	"testing"
)

func TestModuleProgressConcurrentObservationsDoNotInventSuccess(t *testing.T) {
	var p ModuleProgress
	p.Start()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); p.Observe("ATTEMPT"); p.Observe("ISOLATED"); _ = p.Snapshot() }()
	}
	wg.Wait()
	p.Finish("IDLE", nil)
	s := p.Snapshot()
	if s.Attempts != 100 || s.Isolated != 100 || s.Succeeded != 0 || s.LastSuccessUnix != 0 || s.FinishedUnix == 0 {
		t.Fatal(s)
	}
	p.Observe("SUCCEEDED")
	p.Finish("COMPLETED", nil)
	s = p.Snapshot()
	if s.Succeeded != 1 || s.LastSuccessUnix == 0 {
		t.Fatal(s)
	}
	p.Start()
	p.Finish("COMPLETED", errors.New("ledger failed"))
	s = p.Snapshot()
	if s.State != "FAILED" || s.Failures != 1 || s.Succeeded != 1 {
		t.Fatal(s)
	}
	p.Start()
	p.Finish("GATED", nil)
	if p.Snapshot().Succeeded != 1 {
		t.Fatal("gate counted as successful work")
	}
}
