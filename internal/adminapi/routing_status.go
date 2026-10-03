package adminapi

import (
	"encoding/json"
	"net/http"
)

func (s *Server) residentialRoutingStatus(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	err := s.DB.QueryRowContext(r.Context(), `WITH panels AS(
 SELECT p.id FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id
 WHERE p.enabled AND dr.state<>'DELETED' AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=p.droplet_id AND d.state='PANEL_COMPLETE')
 ), progress AS(
 SELECT p.id,c.enabled AND(c.fleet OR p.id=ANY(c.panel_ids)) scoped,
 COALESCE(rs.state='APPLIED' AND rs.revision=c.revision AND rs.verified_at>now()-interval '60 seconds',false) verified,
 COALESCE(rs.state='FAILED',false) failed
 FROM panels p CROSS JOIN residential_routing_control c LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id)
 SELECT json_build_object('enabled',c.enabled,'fleet',c.fleet,'revision',c.revision,
 'total_panels',(SELECT count(*) FROM progress),
 'scoped_panels',(SELECT count(*) FROM progress WHERE scoped),
 'verified_panels',(SELECT count(*) FROM progress WHERE scoped AND verified),
 'failed_panels',(SELECT count(*) FROM progress WHERE scoped AND failed),
 'configured_proxies',(SELECT count(*) FROM residential_proxies),
 'healthy_proxies',(SELECT count(*) FROM residential_proxies rp JOIN proxies p ON p.id=rp.proxy_id WHERE rp.enabled AND p.status='healthy' AND p.last_success_at>now()-interval '3 minutes'))
 FROM residential_routing_control c WHERE singleton`).Scan(&raw)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, json.RawMessage(raw))
}
