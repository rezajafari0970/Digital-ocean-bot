package observability

import (
	"context"
	"database/sql"
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
	}
	return r
}
