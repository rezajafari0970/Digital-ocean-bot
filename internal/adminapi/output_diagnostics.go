package adminapi

import (
	"sync"
	"time"
)

// repeatedOutputDiagnostics bounds repeated status messages while preserving
// every status change. Persistence and database errors never use this limiter.
type repeatedOutputDiagnostics struct {
	mu      sync.Mutex
	entries map[string]outputDiagnostic
}
type outputDiagnostic struct {
	summary string
	at      time.Time
}

func (d *repeatedOutputDiagnostics) shouldLog(key, summary string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries == nil {
		d.entries = make(map[string]outputDiagnostic)
	}
	if previous, ok := d.entries[key]; ok && previous.summary == summary && now.Sub(previous.at) >= 0 && now.Sub(previous.at) < 5*time.Minute {
		return false
	}
	if _, exists := d.entries[key]; !exists && len(d.entries) >= 1024 {
		oldestKey := ""
		var oldest time.Time
		for k, v := range d.entries {
			if oldestKey == "" || v.at.Before(oldest) {
				oldestKey, oldest = k, v.at
			}
		}
		delete(d.entries, oldestKey)
	}
	d.entries[key] = outputDiagnostic{summary: summary, at: now}
	return true
}
