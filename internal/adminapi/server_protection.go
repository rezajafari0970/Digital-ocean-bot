package adminapi

import (
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/serverprotection"
	"net/http"
)

// Last verified blocked admission is sticky until a valid open/disabled receipt.
// Neither a failed/stale poll nor a requested policy change proves recovery.
const serverProtectionOutputPredicate = `
 AND NOT EXISTS(
 SELECT 1 FROM server_protection_nodes pn WHERE pn.panel_id=p.id
 AND (pn.verified_status->>'admission_blocked'='true'
 OR (pn.verified_status->>'enabled'='true' AND pn.verified_status->>'xui_state' IN ('failed','inactive','active_xray_failed')))) `

func (s *Server) serverProtectionStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !residentialAdmin(w, r) {
		return
	}
	var data []byte
	err := s.DB.QueryRowContext(r.Context(), `
 SELECT json_build_object(
 'control',json_build_object('enabled',c.enabled,'scope',c.scope,'panel_ids',c.panel_ids,'revision',c.revision),
 'inherits_future',c.enabled AND c.scope='fleet',
 'sample_interval_ms',250,'telemetry_fresh_seconds',60,
 'capabilities',json_build_object('resource_admission',true,'bounded_service_recovery',true,
 'client_failover',false,'seamless_session_migration',false,'per_user_admission',false),
 'panels',COALESCE((SELECT json_agg(json_build_object(
 'id',p.id,'label',split_part(p.base_url,'/',3),'server_state',d.state,
 'eligible',p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND dp.state='PANEL_COMPLETE' AND d.state IN('READY','EXPIRING') AND(d.expires_at IS NULL OR d.expires_at>now()+interval '1 minute'),
 'assigned',n.panel_id IS NOT NULL,'desired_enabled',n.desired_enabled,
 'desired_revision',n.desired_revision,'applied_revision',n.applied_revision,
 'state',COALESCE(n.state,'UNASSIGNED'),'checked_at',n.checked_at,'error',COALESCE(n.last_error,''),
 'verified',COALESCE(n.control_revision=c.revision AND n.applied_revision=n.desired_revision
 AND n.state=CASE WHEN n.desired_enabled THEN 'APPLIED' ELSE 'DISABLED' END
 AND n.checked_at>now()-interval '60 seconds',false),
 'last_verified_at',n.verified_at,'last_verified_status',COALESCE(n.verified_status,'{}'::jsonb),
 'output_withheld',COALESCE(n.verified_status->>'admission_blocked'='true' OR (n.verified_status->>'enabled'='true' AND n.verified_status->>'xui_state' IN ('failed','inactive','active_xray_failed')),false),
 'status',COALESCE(n.status,'{}'::jsonb)) ORDER BY p.id)
 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
 JOIN deployments dp ON dp.droplet_id=d.id LEFT JOIN server_protection_nodes n ON n.panel_id=p.id
 WHERE d.state<>'DELETED'),'[]'::json))
 FROM server_protection_control c WHERE singleton
 `).Scan(&data)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, json.RawMessage(data))
}
func (s *Server) serverProtectionAction(w http.ResponseWriter, r *http.Request) {
	if !residentialAdmin(w, r) {
		return
	}
	var q serverprotection.Request
	if !decodeResidential(w, r, &q) {
		return
	}
	result, err := (serverprotection.Store{DB: s.DB}).Change(r.Context(), q)
	if err != nil {
		switch {
		case errors.Is(err, serverprotection.ErrConflict):
			writeJSON(w, 409, map[string]string{"error": "protection_conflict", "detail": err.Error()})
		case errors.Is(err, serverprotection.ErrInvalid):
			writeJSON(w, 400, map[string]string{"error": "invalid_protection_request", "detail": err.Error()})
		default:
			writeJSON(w, 500, errorBody())
		}
		return
	}
	writeJSON(w, 202, result)
}
