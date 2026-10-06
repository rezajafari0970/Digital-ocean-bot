package serverprotection

import (
	"fmt"
	"math"
	"time"
)

type Engine struct {
	criticalSince, healthySince time.Time
	blocked                     bool
}

func (e *Engine) Evaluate(m Metrics, now time.Time) (string, string, bool) {
	reason := ""
	emergency := m.MemTotalMB > 0 && m.MemAvailableMB < math.Max(32, m.MemTotalMB*.04)
	switch {
	case emergency:
		reason = "memory reserve exhausted"
	case m.MemTotalMB > 0 && m.MemAvailableMB < math.Max(64, m.MemTotalMB*.10) && m.MemoryPSI >= 10:
		reason = "memory pressure"
	case m.CPUHotCorePercent >= 95 && m.CPUPSI >= 20:
		reason = "CPU queue pressure"
	case m.FDPercent >= 90:
		reason = "file descriptor pressure"
	case m.ConntrackSupported && m.ConntrackPercent >= 90:
		reason = "connection table pressure"
	case m.DiskFreeMB < 32 && m.IOPSI >= 10:
		reason = "disk pressure"
	case m.InodeFreePercent >= 0 && m.InodeFreePercent < .5:
		reason = "inode reserve exhausted"
	}
	if reason != "" {
		e.healthySince = time.Time{}
		if e.criticalSince.IsZero() {
			e.criticalSince = now
		}
		if emergency || e.blocked || now.Sub(e.criticalSince) >= 2*time.Second {
			e.blocked = true
			return "CRITICAL", reason, true
		}
		return "WARM", reason, false
	}
	e.criticalSince = time.Time{}
	if e.blocked {
		if e.healthySince.IsZero() {
			e.healthySince = now
		}
		if now.Sub(e.healthySince) < 30*time.Second {
			return "RECOVERING", "waiting for 30 seconds of stable resources", true
		}
		e.blocked = false
	}
	if m.MemTotalMB > 0 && m.MemAvailableMB < m.MemTotalMB*.20 || m.CPUPSI >= 10 || m.CPUStealPercent >= 15 || m.IOPSI >= 5 {
		return "WARM", "reduced resource headroom", false
	}
	return "READY", "resources within initial conservative limits", false
}
func (s Status) ValidateReceipt(p Policy) error {
	if s.Revision != p.Revision || s.Enabled != p.Enabled || s.Version != Version {
		return fmt.Errorf("policy receipt mismatch")
	}
	if s.SampleAgeMS < 0 || s.SampleAgeMS > 5000 {
		return fmt.Errorf("stale local sample")
	}
	if p.Enabled && (!s.AgentRunning || !s.NFTSupported || len(s.Ports) == 0) {
		return fmt.Errorf("local protection not ready: %s", s.Reason)
	}
	if p.Enabled {
		switch s.State {
		case "READY", "WARM":
			if s.AdmissionBlocked {
				return fmt.Errorf("inconsistent open admission receipt")
			}
		case "CRITICAL", "RECOVERING":
			if !s.AdmissionBlocked {
				return fmt.Errorf("admission protection not verified")
			}
		default:
			return fmt.Errorf("local protection state %s: %s", s.State, s.Reason)
		}
	}
	if !p.Enabled && (s.AgentRunning || s.AdmissionBlocked || s.State != "DISABLED") {
		return fmt.Errorf("disable cleanup not verified")
	}
	return nil
}
