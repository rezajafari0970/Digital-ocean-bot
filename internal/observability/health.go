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
		var workerOK bool
		err = h.DB.QueryRowContext(checkCtx, `SELECT EXISTS(SELECT 1 FROM worker_heartbeats WHERE kind='production' AND last_seen_at>now()-interval '30 seconds'
 AND (metadata->>'recovery_scan_unix')::bigint>extract(epoch FROM now()-interval '2 minutes')
 AND (metadata->>'scheduler_scan_unix')::bigint>extract(epoch FROM now()-interval '5 minutes')
 AND (metadata->>'lifecycle_scan_unix')::bigint>extract(epoch FROM now()-interval '5 minutes'))`).Scan(&workerOK)
		if err != nil {
			workerOK = false
		}
		check := Check{Name: "worker_progress", OK: workerOK}
		if !workerOK {
			check.Message = "worker heartbeat or recovery/scheduler/lifecycle progress stale"
			r.Status = "not_ready"
		}
		r.Checks = append(r.Checks, check)
		var metadata []byte
		err = h.DB.QueryRowContext(checkCtx, `SELECT metadata FROM worker_heartbeats WHERE kind='production' AND last_seen_at>now()-interval '30 seconds' ORDER BY last_seen_at DESC LIMIT 1`).Scan(&metadata)
		progressOK := err == nil && clientProgressReady(metadata, time.Now())
		lanesOK := err == nil && lifecycleLanesReady(metadata)
		lanesCheck := Check{Name: "lifecycle_account_lanes", OK: lanesOK}
		if !lanesOK {
			lanesCheck.Message = "lifecycle lane progress unavailable or deadline not honored"
			r.Status = "not_ready"
		}
		r.Checks = append(r.Checks, lanesCheck)
		progressCheck := Check{Name: "client_mutation_progress", OK: progressOK}
		if !progressOK {
			progressCheck.Message = "client mutation scan unavailable, failed or stalled"
			r.Status = "not_ready"
		}
		r.Checks = append(r.Checks, progressCheck)
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
