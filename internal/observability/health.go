package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}
type Report struct {
	Status string    `json:"status"`
	Checks []Check   `json:"checks"`
	At     time.Time `json:"at"`
}
type Health struct {
	DB            *sql.DB
	RequireWorker bool
	WorkerMode    string
}

func (h Health) Readiness(ctx context.Context) Report {
	r := Report{Status: "ready", At: time.Now().UTC()}
	if h.DB == nil {
		r.Status = "not_ready"
		r.Checks = append(r.Checks, Check{Name: "database", OK: false, Message: "not configured"})
		return r
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := h.DB.PingContext(checkCtx)
	ok := err == nil
	c := Check{Name: "database", OK: ok}
	if err != nil {
		c.Message = "unavailable"
		r.Status = "not_ready"
	}
	r.Checks = append(r.Checks, c)
	if ok && h.RequireWorker {
		controlKind, panelKind := "production", "production"
		switch h.WorkerMode {
		case "", "all":
		case "split":
			controlKind, panelKind = "production-control", "production-panels"
		default:
			r.Status = "not_ready"
			r.Checks = append(r.Checks, Check{Name: "worker_topology", OK: false, Message: "invalid worker mode"})
			return r
		}
		now := time.Now()
		control, controlErr := h.workerMetadata(checkCtx, controlKind)
		panel, panelErr := control, controlErr
		if panelKind != controlKind {
			panel, panelErr = h.workerMetadata(checkCtx, panelKind)
		}
		add := func(name string, good bool, message string) {
			c := Check{Name: name, OK: good}
			if !good {
				c.Message = message
				r.Status = "not_ready"
			}
			r.Checks = append(r.Checks, c)
		}
		add("worker_progress", controlErr == nil && controlProgressReady(control, now) && supervisionReady(control), "control worker heartbeat or recovery/scheduler/lifecycle progress stale")
		if h.WorkerMode == "split" {
			add("panel_worker_progress", panelErr == nil && supervisionReady(panel), "panel worker heartbeat or supervised progress unavailable")
		}
		add("lifecycle_account_lanes", controlErr == nil && lifecycleLanesReady(control), "lifecycle lane progress unavailable or deadline not honored")
		add("client_mutation_progress", panelErr == nil && clientProgressReady(panel, now), "client mutation scan unavailable, failed or stalled")
	}

	return r
}

// Scanner liveness does not assert mutation success or override a policy gate.
func clientProgressReady(metadata []byte, now time.Time) bool {
	var m struct {
		Client struct {
			State    string `json:"state"`
			Started  int64  `json:"started_unix"`
			Finished int64  `json:"finished_unix"`
		} `json:"client_mutation"`
	}
	if json.Unmarshal(metadata, &m) != nil {
		return false
	}
	fresh := func(v int64) bool {
		return v > 0 && v > now.Add(-2*time.Minute).Unix() && v <= now.Add(30*time.Second).Unix()
	}
	switch m.Client.State {
	case "RUNNING":
		return fresh(m.Client.Started)
	case "IDLE", "GATED", "COMPLETED":
		return fresh(m.Client.Finished)
	default:
		return false
	}
}

func lifecycleLanesReady(metadata []byte) bool {
	var m struct {
		Lanes *struct {
			InFlight int `json:"in_flight"`
			Stalled  int `json:"stalled"`
		} `json:"lifecycle_lanes"`
	}
	if json.Unmarshal(metadata, &m) != nil || m.Lanes == nil {
		return false
	}
	return m.Lanes.InFlight >= 0 && m.Lanes.Stalled == 0
}

// Select one fresh process consistently. Split mode deliberately cannot fall
// back to a legacy monolith when a required role disappears.
func (h Health) workerMetadata(ctx context.Context, kind string) ([]byte, error) {
	var metadata []byte
	err := h.DB.QueryRowContext(ctx, `SELECT metadata FROM worker_heartbeats
 WHERE kind=$1 AND last_seen_at>now()-interval '30 seconds'
 AND last_seen_at<=now()+interval '30 seconds'
 ORDER BY last_seen_at DESC,worker_id DESC LIMIT 1`, kind).Scan(&metadata)
	return metadata, err
}
func controlProgressReady(raw []byte, now time.Time) bool {
	var m struct {
		Recovery  int64 `json:"recovery_scan_unix"`
		Scheduler int64 `json:"scheduler_scan_unix"`
		Lifecycle int64 `json:"lifecycle_scan_unix"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	fresh := func(v int64, age time.Duration) bool {
		return v > now.Add(-age).Unix() && v <= now.Add(30*time.Second).Unix()
	}
	return fresh(m.Recovery, 2*time.Minute) && fresh(m.Scheduler, 5*time.Minute) && fresh(m.Lifecycle, 5*time.Minute)
}

// A fresh heartbeat cannot substitute for the complete registered module set.
func supervisionReady(raw []byte) bool {
	var m struct {
		Modules     []string `json:"modules"`
		Supervision struct {
			Version int  `json:"version"`
			Healthy bool `json:"healthy"`
			Modules []struct {
				Name    string `json:"name"`
				State   string `json:"state"`
				Active  int    `json:"active"`
				Waiting int    `json:"waiting"`
			} `json:"modules"`
		} `json:"supervision"`
	}
	if json.Unmarshal(raw, &m) != nil || m.Supervision.Version != 1 || !m.Supervision.Healthy || len(m.Modules) == 0 || len(m.Supervision.Modules) != len(m.Modules) {
		return false
	}
	names := map[string]bool{}
	for _, n := range m.Modules {
		if n == "" || names[n] {
			return false
		}
		names[n] = true
	}
	for _, x := range m.Supervision.Modules {
		if !names[x.Name] || x.Active < 0 || x.Waiting < 0 {
			return false
		}
		if x.State != "IDLE" && x.State != "RUNNING" && x.State != "WAITING" {
			return false
		}
		delete(names, x.Name)
	}
	return len(names) == 0
}
