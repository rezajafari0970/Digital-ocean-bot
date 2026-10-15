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
 'funding',(SELECT to_jsonb(f) FROM ip_account_funding f WHERE f.account_id=a.id),
 'reserved_commitment_micros',(SELECT COALESCE(sum(fr.commitment_micros),0) FROM ip_funding_reservations fr JOIN ip_mutation_intents m ON m.id=fr.operation_id WHERE m.account_id=a.id AND fr.settled_at IS NULL AND m.state NOT IN('REJECTED','CANCELLED')),
 'policies',COALESCE((SELECT jsonb_agg(jsonb_build_object('family',p.family,'revision',p.revision,'policy',p.policy) ORDER BY p.family) FROM ip_policies p WHERE p.account_id=a.id),'[]'::jsonb),
 'operations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'family',m.family,'product',m.product,'server_id',m.server_id,'action',m.action,'state',m.state,'created_at',m.created_at,'dispatched_at',m.dispatched_at,'next_attempt_at',m.next_attempt_at,'error_class',m.error_class) ORDER BY m.created_at DESC) FROM (SELECT * FROM ip_mutation_intents WHERE account_id=a.id ORDER BY created_at DESC LIMIT 50) m),'[]'::jsonb),
 'managed_allocations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',i.id,'family',i.family,'product',i.product,'server_id',i.server_id,'prefix',i.parent_prefix,'state',i.state,'acquired_at',i.acquired_at,'currency',i.currency,'hourly_micros',i.hourly_micros,'cost_source',i.cost_source,'cost_checked_at',i.cost_checked_at) ORDER BY i.acquired_at DESC) FROM ip_allocations i WHERE i.account_id=a.id AND i.state<>'RELEASED'),'[]'::jsonb),
 'endpoints',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',e.id,'address',host(e.address),'prefix',e.diversity_prefix,'server_id',e.server_id,'phase',e.phase,'acquired_at',e.acquired_at,'expires_at',e.expires_at,'protected_leases',(SELECT count(*) FROM ip_publication_leases l WHERE l.endpoint_id=e.id AND l.migrated_at IS NULL AND (l.valid_until IS NULL OR l.valid_until>now()))) ORDER BY e.acquired_at DESC) FROM ip_endpoints e WHERE e.account_id=a.id AND e.retired_at IS NULL),'[]'::jsonb),
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
	writeJSON(w, 200, map[string]any{"stage": "LIFECYCLE_PREPARATION", "allocation_enabled": false, "rotation_enabled": false, "network_changes_enabled": false, "accounts": out,
		"blockers": []string{"OPERATIONAL_SETTINGS_UNSET", "CANARY_APPROVAL_REQUIRED", "EXECUTION_NETWORK_OUTPUT_INTEGRATION_PENDING"},
		"stages":   map[string]string{"inventory": "implemented", "policies": "implemented", "journal": "implemented_offline", "ownership": "implemented_offline", "planning": "unit_tested", "allocation": "not_started", "attachment": "not_started", "network": "not_started", "health": "not_started", "publication": "not_started", "rotation": "not_started", "release": "not_started"},
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
