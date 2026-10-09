package adminapi

import (
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net/http"
)

func (s *Server) residentialPerformanceStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !residentialAdmin(w, r) {
		return
	}
	var b []byte
	err := s.DB.QueryRowContext(r.Context(), `SELECT json_build_object(
 'defaults',$1::jsonb,'revision',c.revision,'routing_enabled',c.enabled,
 'panels',COALESCE((SELECT json_agg(json_build_object('id',p.id,'label',split_part(p.base_url,'/',3),'plan_hash',rs.plan_hash,
 'state',COALESCE(rs.state,d.state),'server_state',d.state,'verified_at',rs.verified_at,'expires_at',d.expires_at,'config',pp.config,'experiment_id',pp.experiment_id,
 'eligible',COALESCE(c.enabled AND(c.fleet OR p.id=ANY(c.panel_ids)) AND rs.pool_enabled AND rs.state='APPLIED' AND rs.revision=c.revision AND rs.verified_at>now()-interval '60 seconds' AND (d.expires_at IS NULL OR d.expires_at>now()+interval '10 minutes') AND EXISTS(SELECT 1 FROM residential_proxies WHERE enabled AND status='healthy' AND last_success_at>now()-interval '3 minutes'),false)) ORDER BY p.id)
 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id LEFT JOIN residential_performance_panels pp ON pp.panel_id=p.id
 WHERE p.enabled AND d.state NOT IN('DELETED','DELETING','RETIRING') AND(d.expires_at IS NULL OR d.expires_at>now()) AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL),'[]'::json),
 'experiment',(SELECT json_build_object('id',e.id,'state',e.state,'version',e.version,'config',e.spec,'deadline',e.deadline,'reason',e.reason,'mode',e.duration_mode,'scope',e.publish_scope,'active_tuning',e.tuning,'inherits_future',e.state='KEPT' AND e.duration_mode='permanent' AND e.publish_scope='fleet',
 'can_keep',e.state='RUNNING' AND e.deadline>now() AND NOT EXISTS(SELECT 1 FROM residential_performance_targets t LEFT JOIN residential_performance_panels pp ON pp.panel_id=t.panel_id LEFT JOIN panel_routing_state rs ON rs.panel_id=t.panel_id WHERE t.experiment_id=e.id AND(t.state<>'APPLIED' OR pp.applied_generation IS DISTINCT FROM t.generation OR pp.verified_at IS NULL OR pp.verified_at<now()-interval '60 seconds' OR rs.state IS DISTINCT FROM 'APPLIED' OR rs.revision IS DISTINCT FROM c.revision OR rs.performance_generation IS DISTINCT FROM t.generation)),
 'counts',(SELECT json_build_object('total',count(*),'retired',count(*) FILTER(WHERE t.state='RETIRED'),'verified',count(*) FILTER(WHERE t.state='APPLIED' AND pp.experiment_id=t.experiment_id AND pp.applied_generation=t.generation AND pp.verified_at>now()-interval '60 seconds' AND rs.state='APPLIED' AND rs.revision=c.revision AND rs.performance_generation=t.generation)) FROM residential_performance_targets t LEFT JOIN residential_performance_panels pp ON pp.panel_id=t.panel_id LEFT JOIN panel_routing_state rs ON rs.panel_id=t.panel_id WHERE t.experiment_id=e.id),
 'targets',COALESCE((SELECT json_agg(json_build_object('panel_id',t.panel_id,'state',t.state,'tuning_id',t.tuning_id,'tuning_generation',t.tuning_generation,'failures',t.failures,'error',pp.last_error,'verified_at',pp.verified_at,'verified',COALESCE(pp.experiment_id=t.experiment_id AND pp.applied_generation=t.generation AND pp.verified_at>now()-interval '60 seconds' AND rs.state='APPLIED' AND rs.revision=c.revision AND rs.performance_generation=t.generation,false)) ORDER BY t.panel_id) FROM (SELECT * FROM residential_performance_targets WHERE experiment_id=e.id ORDER BY CASE WHEN state='RETIRED' THEN 1 ELSE 0 END,panel_id LIMIT 200) t LEFT JOIN residential_performance_panels pp ON pp.panel_id=t.panel_id LEFT JOIN panel_routing_state rs ON rs.panel_id=t.panel_id),'[]'::json))
 FROM residential_performance_experiments e ORDER BY e.created_at DESC LIMIT 1))
 FROM residential_routing_control c WHERE singleton`, mustPerformanceJSON(residentialperf.Balanced())).Scan(&b)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, json.RawMessage(b))
}
func mustPerformanceJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func (s *Server) residentialPerformanceAction(w http.ResponseWriter, r *http.Request) {
	if !residentialAdmin(w, r) {
		return
	}
	var q residentialperf.Request
	if !decodeResidential(w, r, &q) {
		return
	}
	result, err := (residentialperf.Store{DB: s.DB}).Do(r.Context(), q)
	if err != nil {
		var e *residentialperf.Error
		if errors.As(err, &e) {
			writeJSON(w, e.Code, map[string]string{"error": "performance_conflict", "detail": e.Message})
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}
	writeJSON(w, 200, result)
}
