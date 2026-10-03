package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type clientMutationRequest struct {
	AccountID  string  `json:"account_id"`
	PanelID    string  `json:"panel_id"`
	InboundID  int64   `json:"inbound_id"`
	ClientID   string  `json:"client_id"`
	Email      string  `json:"email,omitempty"`
	Enable     *bool   `json:"enable,omitempty"`
	TotalGB    *int64  `json:"total_gb,omitempty"`
	ExpiryTime *int64  `json:"expiry_time,omitempty"`
	LimitIP    *int    `json:"limit_ip,omitempty"`
	Flow       *string `json:"flow,omitempty"`
}

func clientMutationKind(r *http.Request) (clientops.Kind, bool) {
	switch r.Method {
	case http.MethodPost:
		return clientops.KindCreate, true
	case http.MethodPatch:
		return clientops.KindUpdate, true
	case http.MethodDelete:
		return clientops.KindDelete, true
	default:
		return "", false
	}
}

func (s *Server) clientMutations(w http.ResponseWriter, r *http.Request) {
	kind, ok := clientMutationKind(r)
	if !ok {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key_required"})
		return
	}
	var req clientMutationRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if req.AccountID == "" || req.PanelID == "" || req.InboundID <= 0 || req.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	var eligible bool
	if err := s.DB.QueryRowContext(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id WHERE p.id=$1 AND p.account_id=$2 AND p.enabled=true AND a.enabled=true AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND EXISTS(SELECT 1 FROM panel_inbound_inventory i WHERE i.panel_id=p.id AND i.remote_id=$3 AND i.present=true AND i.enabled=true))",
		req.PanelID, req.AccountID, req.InboundID,
	).Scan(&eligible); err != nil || !eligible {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "panel_not_mutable"})
		return
	}

	var payload any
	switch kind {
	case clientops.KindCreate:
		if req.Email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email_required"})
			return
		}
		enable := true
		if req.Enable != nil {
			enable = *req.Enable
		}
		var total int64
		if req.TotalGB != nil {
			total = *req.TotalGB
		}
		var expiry int64
		if req.ExpiryTime != nil {
			expiry = *req.ExpiryTime
		}
		var limit int
		if req.LimitIP != nil {
			limit = *req.LimitIP
		}
		flow := "xtls-rprx-vision"
		if req.Flow != nil {
			flow = *req.Flow
		}
		payload = map[string]any{"Client": sanaei.Client{
			ID: req.ClientID, Email: req.Email, Enable: enable,
			TotalGB: total, ExpiryTime: expiry, LimitIP: limit, Flow: flow,
		}}
	case clientops.KindUpdate:
		patch := clientops.ClientPatch{
			Email: reqString(req.Email), Enable: req.Enable, TotalGB: req.TotalGB,
			ExpiryTime: req.ExpiryTime, LimitIP: req.LimitIP, Flow: req.Flow,
		}
		if patch.Empty() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_patch"})
			return
		}
		payload = map[string]any{"Patch": patch}
	case clientops.KindDelete:
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	job, created, err := (clientops.Journal{DB: s.DB}).Reserve(r.Context(), clientops.Request{
		AccountID: req.AccountID, PanelID: req.PanelID, InboundID: req.InboundID,
		ClientID: req.ClientID, Kind: kind, IdempotencyKey: key, Payload: raw,
	})
	if errors.Is(err, clientops.ErrIdempotencyConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency_conflict"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "mutation_not_queued"})
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusAccepted
	}
	gate, gateErr := (clientops.Journal{DB: s.DB}).Gate(r.Context())
	executionEnabled := gateErr == nil && gate.Enabled && !gate.KillSwitch && gate.Concurrency == 1
	writeJSON(w, code, map[string]any{
		"id": job.ID, "state": job.State, "created": created,
		"execution_enabled": executionEnabled,
	})
}

func (s *Server) clientMutationStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	job, err := (clientops.Journal{DB: s.DB}).Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func reqString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *Server) listClientMutations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(),
		"SELECT id::text,kind,client_id,state,attempts,last_error,created_at,updated_at,completed_at FROM client_mutation_jobs ORDER BY created_at DESC LIMIT 20")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	defer rows.Close()
	recent := make([]map[string]any, 0, 20)
	for rows.Next() {
		var id, kind, clientID, state, lastError string
		var attempts int
		var created, updated time.Time
		var completed sql.NullTime
		if rows.Scan(&id, &kind, &clientID, &state, &attempts, &lastError, &created, &updated, &completed) != nil {
			continue
		}
		item := map[string]any{"id": id, "kind": kind, "client_id": clientID, "state": state,
			"attempts": attempts, "last_error": lastError, "created_at": created, "updated_at": updated}
		if completed.Valid {
			item["completed_at"] = completed.Time
		}
		recent = append(recent, item)
	}
	counts := map[string]int{"PENDING": 0, "RUNNING": 0, "SUCCEEDED": 0, "FAILED": 0, "OBSOLETE": 0}
	countRows, err := s.DB.QueryContext(r.Context(), "SELECT state,count(*) FROM client_mutation_jobs GROUP BY state")
	if err == nil {
		defer countRows.Close()
		for countRows.Next() {
			var state string
			var count int
			if countRows.Scan(&state, &count) == nil {
				counts[state] = count
			}
		}
	}
	gate, gateErr := (clientops.Journal{DB: s.DB}).Gate(r.Context())
	executionEnabled := gateErr == nil && gate.Enabled && !gate.KillSwitch && gate.Concurrency == 1
	writeJSON(w, http.StatusOK, map[string]any{
		"execution_enabled": executionEnabled,
		"kill_switch":       gate.KillSwitch,
		"scope_panel_id":    gate.PanelID,
		"scope_inbound_id":  gate.InboundID,
		"counts":            counts, "recent": recent,
	})
}
