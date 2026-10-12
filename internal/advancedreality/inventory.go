package advancedreality

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// InventorySQL reuses existing native observations and performs no panel
// requests. Only explicitly selected aggregate fields can leave this boundary.
// The DB transaction supplies a consistent census with a bounded statement.
const InventorySQL = `SELECT json_build_object(
 'observed_at',now(), 'mode','OBSERVE_ONLY', 'version',$1::text,
 'enabled',true, 'can_apply',false,
 'panels',COALESCE((SELECT json_agg(json_build_object(
   'id',p.id, 'panel_version',NULLIF(p.version,''), 'core_version',NULL,
   'server_state',d.state, 'panel_enabled',p.enabled,
   'account_enabled',a.enabled, 'account_state',a.provider_state,
   'route_state',COALESCE(rs.state,'UNKNOWN'),
   'route_observed_at',rs.verified_at,
   'route_fresh',COALESCE(rs.state='APPLIED' AND rs.verified_at BETWEEN now()-interval '60 seconds' AND now(),false),
   'os_observed_at',n.verified_status->'observed_at',
   'os_verified_at',n.verified_at,
   'os_metrics_fresh',COALESCE(n.state IN('APPLIED','DISABLED') AND n.verified_at BETWEEN now()-interval '60 seconds' AND now()
      AND n.verified_status->>'agent_running'='true'
      AND n.verified_status->>'sample_age_ms' ~ '^[0-9]{1,6}$'
      AND (CASE WHEN n.verified_status->>'sample_age_ms' ~ '^[0-9]{1,6}$' THEN (n.verified_status->>'sample_age_ms')::int ELSE 999999 END)<60000,false),
   'os_last_observation',CASE WHEN n.verified_at IS NOT NULL THEN json_build_object(
       'cpu_percent',n.verified_status->'metrics'->'cpu_percent',
       'mem_total_mb',n.verified_status->'metrics'->'mem_total_mb',
       'mem_available_mb',n.verified_status->'metrics'->'mem_available_mb',
       'tx_mbps',n.verified_status->'metrics'->'tx_mbps',
       'rx_mbps',n.verified_status->'metrics'->'rx_mbps',
       'process_fd_percent',CASE WHEN n.verified_status->'metrics'->>'process_fd_known'='true' THEN n.verified_status->'metrics'->'process_fd_percent' END) END,
   'sni_last_observation',(SELECT json_build_object('observed_at',s.scanned_at,'tls_version',s.tls_version,
       'cert_valid',s.cert_valid,'cert_chain_valid',s.cert_chain_valid,'latency_ms',s.latency_ms,
       'source','existing_reality_scorer','selection_changed',false) FROM reality_target_selections s WHERE s.panel_id=p.id),
   'inbounds',COALESCE((SELECT json_agg(json_build_object('id',i.remote_id,'protocol',i.protocol,'transport',i.transport,
       'security',i.security,'port',i.port,'enabled',i.enabled,'client_count',i.client_count,
       'observed_at',i.last_seen_at,'fresh',i.last_seen_at BETWEEN now()-interval '60 seconds' AND now()) ORDER BY i.remote_id)
       FROM panel_inbound_inventory i WHERE i.panel_id=p.id AND i.present),'[]'::json)) ORDER BY p.id)
   FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
   LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id LEFT JOIN server_protection_nodes n ON n.panel_id=p.id
   WHERE d.state<>'DELETED'),'[]'::json))`

func Inventory(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SET LOCAL statement_timeout='5000ms'`); e != nil {
		return nil, e
	}
	var b []byte
	if e = tx.QueryRowContext(ctx, InventorySQL, Version).Scan(&b); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	// Verification of an old agent sample must not renew that sample's age.
	var result map[string]any
	if e = json.Unmarshal(b, &result); e != nil {
		return nil, e
	}
	now := time.Now()
	if panels, ok := result["panels"].([]any); ok {
		for _, v := range panels {
			p, ok := v.(map[string]any)
			if !ok {
				continue
			}
			s, _ := p["os_observed_at"].(string)
			at, err := time.Parse(time.RFC3339Nano, s)
			if err != nil || at.After(now) || now.Sub(at) > 60*time.Second {
				p["os_metrics_fresh"] = false
			}
		}
	}
	return json.Marshal(result)
}
