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
type Health struct{ DB *sql.DB }

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
	return r
}
