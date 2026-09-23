package adminapi

import (
	"encoding/json"
	"net/http"
)

type scheduleWrite struct {
	AccountID       string `json:"account_id"`
	ProfileID       string `json:"profile_id"`
	IntervalSeconds int    `json:"interval_seconds"`
	BatchSize       int    `json:"batch_size"`
	MaxConcurrent   int    `json:"max_concurrent"`
	Enabled         *bool  `json:"enabled"`
}

func (s *Server) upsertSchedule(w http.ResponseWriter, r *http.Request) {
	var x scheduleWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.AccountID == "" || x.ProfileID == "" || x.IntervalSeconds < 60 || x.BatchSize < 1 || x.MaxConcurrent < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	enabled := true
	if x.Enabled != nil {
		enabled = *x.Enabled
	}
	var id string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO schedules(id,account_id,profile_id,enabled,interval_seconds,batch_size,max_concurrent,next_run_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,now()) ON CONFLICT(account_id,profile_id) DO UPDATE SET enabled=EXCLUDED.enabled,interval_seconds=EXCLUDED.interval_seconds,batch_size=EXCLUDED.batch_size,max_concurrent=EXCLUDED.max_concurrent,updated_at=now() RETURNING id::text`, x.AccountID, x.ProfileID, enabled, x.IntervalSeconds, x.BatchSize, x.MaxConcurrent).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	writeJSON(w, 200, map[string]string{"id": id})
}
func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,account_id::text,profile_id::text,enabled,interval_seconds,batch_size,max_concurrent,next_run_at,last_run_at FROM schedules ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, a, p string
		var enabled bool
		var interval, batch, max int
		var next, last any
		if rows.Scan(&id, &a, &p, &enabled, &interval, &batch, &max, &next, &last) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "account_id": a, "profile_id": p, "enabled": enabled, "interval_seconds": interval, "batch_size": batch, "max_concurrent": max, "next_run_at": next, "last_run_at": last})
	}
	writeJSON(w, 200, out)
}
