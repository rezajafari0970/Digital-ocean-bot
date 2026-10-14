package adminapi

import (
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"net/http"
	"time"
)

func (s *Server) ipInventory(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT jsonb_build_object('account_id',a.id,'name',a.name,'provider',a.provider,'enabled',a.enabled,
 'state',CASE WHEN l.state='READING' AND l.lease_until<clock_timestamp() THEN 'INTERRUPTED' ELSE COALESCE(l.state,'NOT_STARTED') END,
 'error_class',COALESCE(l.error_class,''),'http_status',l.http_status,'last_good_at',l.last_good_at,'last_attempt_at',l.started_at,'next_attempt_at',GREATEST(l.next_attempt_at,l.rate_limit_until),'rate_limit_until',l.rate_limit_until,
 'resources',COALESCE(l.last_good->'resources','[]'::jsonb),'quotas',COALESCE((SELECT jsonb_agg(jsonb_build_object('family',q.family,'product',q.product,'scope',q.scope,'limit',q.current_limit,'used',q.used,'remaining',CASE WHEN q.current_limit IS NOT NULL AND q.used IS NOT NULL THEN greatest(0,q.current_limit-q.used) END,'source',q.source,'observed_at',q.observed_at,'last_known_limit',q.last_known_limit,'last_known_at',q.last_known_at,'last_known_source',q.last_known_source,'capacity_changed',q.capacity_changed) ORDER BY q.family,q.product,q.scope) FROM ip_quota_evidence q WHERE q.account_id=a.id),'[]'::jsonb))
 FROM accounts a LEFT JOIN ip_inventory_lanes l ON l.account_id=a.id WHERE a.deleted_at IS NULL ORDER BY a.provider,a.name,a.id`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		out = append(out, json.RawMessage(b))
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"stage": "READ_ONLY_FOUNDATION", "allocation_enabled": false, "rotation_enabled": false, "network_changes_enabled": false, "accounts": out,
		"blockers": []string{"OPERATIONAL_SETTINGS_UNSET", "CANARY_APPROVAL_REQUIRED", "MUTATION_NETWORK_OUTPUT_INTEGRATION_PENDING"},
		"stages":   map[string]string{"inventory": "implemented", "planning": "unit_tested", "allocation": "not_started", "attachment": "not_started", "network": "not_started", "health": "not_started", "publication": "not_started", "rotation": "not_started", "release": "not_started"},
		"cost":     map[string]any{"actual_billed": nil, "estimated": nil}, "observed_at": time.Now().UTC()})
}
func (s *Server) refreshIPInventory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !accountOutputID.MatchString(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid_account_id"})
		return
	}
	var enabled bool
	if e := s.DB.QueryRowContext(r.Context(), "SELECT enabled FROM accounts WHERE id=$1 AND deleted_at IS NULL", id).Scan(&enabled); e != nil {
		writeJSON(w, 404, map[string]string{"error": "account_not_found"})
		return
	}
	if !enabled {
		writeJSON(w, 409, map[string]string{"error": "account_disabled"})
		return
	}

	st := ipam.InventoryStore{DB: s.DB}
	result, e := st.Refresh(r.Context(), id, s.Container.ReadIPInventory)
	if errors.Is(e, ipam.ErrLaneBusy) {
		writeJSON(w, 409, map[string]string{"error": "ip_inventory_busy_or_cooldown"})
		return
	}
	if e != nil {
		writeJSON(w, 503, map[string]string{"error": "ip_inventory_receipt_uncommitted"})
		return
	}
	writeJSON(w, 200, result)
}
