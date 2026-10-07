package worker

import (
	"sync"
	"time"
)

// ModuleProgress is observational only. A scan is not proof of a completed
// mutation, and policy-gated/idle modules remain distinguishable from success.
type ModuleSnapshot struct {
	State           string `json:"state"`
	StartedUnix     int64  `json:"started_unix"`
	FinishedUnix    int64  `json:"finished_unix"`
	LastAttemptUnix int64  `json:"last_attempt_unix"`
	LastSuccessUnix int64  `json:"last_success_unix"`
	Attempts        uint64 `json:"attempts"`
	Succeeded       uint64 `json:"succeeded"`
	PolicyDeferred  uint64 `json:"policy_deferred"`
	Isolated        uint64 `json:"isolated"`
	Failures        uint64 `json:"failures"`
}
type ModuleProgress struct {
	mu       sync.Mutex
	snapshot ModuleSnapshot
}

func (p *ModuleProgress) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshot.State = "RUNNING"
	p.snapshot.StartedUnix = time.Now().Unix()
}
func (p *ModuleProgress) Finish(state string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshot.FinishedUnix = time.Now().Unix()
	p.snapshot.State = state
	if err != nil {
		p.snapshot.State = "FAILED"
		p.snapshot.Failures++
	}
}
func (p *ModuleProgress) Observe(outcome string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch outcome {
	case "ATTEMPT":
		p.snapshot.LastAttemptUnix = time.Now().Unix()
		p.snapshot.Attempts++
	case "SUCCEEDED":
		p.snapshot.LastSuccessUnix = time.Now().Unix()
		p.snapshot.Succeeded++
	case "POLICY_DEFERRED":
		p.snapshot.PolicyDeferred++
	case "ISOLATED":
		p.snapshot.Isolated++
	}
}
func (p *ModuleProgress) Snapshot() ModuleSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshot
}
