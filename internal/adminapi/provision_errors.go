package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
	"net/http"
)

// Current attribution follows the latest error of the still-failed step and
// selected installer generation. Earlier probes and retired generations remain
// searchable history without being reported as current failures.
const provisionErrorsSQL = `WITH current_steps AS (
 SELECT pr.id run_id, st.step, st.last_started_at
 FROM provision_runs pr JOIN droplets dr ON dr.id=pr.droplet_id AND dr.account_id=pr.account_id
 JOIN provision_step_attempts st ON st.run_id=pr.id
 WHERE pr.state<>'COMPLETED' AND dr.state<>'DELETED' AND COALESCE(st.last_error,'')<>''
 AND (st.step=pr.current_step OR EXISTS (
  SELECT 1 FROM installer_runs ir JOIN installers i ON i.id=ir.installer_id
  JOIN deployments d ON d.id=ir.deployment_id AND d.account_id=pr.account_id AND d.droplet_id=pr.droplet_id
  WHERE ir.provision_run_id=pr.id AND ir.state IN ('INSTALLING','VERIFYING','FAILED','ROLLBACK_REQUIRED')
  AND ir.generation=d.installer_generation
  AND starts_with(st.step,'installer-g'||ir.generation||'-'||i.name||'-v'||i.version||'-')
 ))
), missing_installer_runs AS (
 SELECT pr.id run_id,pr.current_step FROM provision_runs pr
 JOIN droplets dr ON dr.id=pr.droplet_id AND dr.account_id=pr.account_id
 WHERE pr.state='WAITING_INSTALLER' AND dr.state<>'DELETED' AND COALESCE(pr.last_error,'') IN ('installer not configured','INSTALLER_NOT_CONFIGURED')
 -- Older workers used this message for configured handoffs as well. Verify
 -- actual selection rather than treating their legacy message as proof.
 AND NOT EXISTS (
  SELECT 1 FROM deployments d WHERE d.account_id=pr.account_id AND d.droplet_id=pr.droplet_id
  AND (EXISTS(SELECT 1 FROM deployment_installer_selections sel WHERE sel.deployment_id=d.id AND sel.generation=d.installer_generation)
   OR (d.installer_generation=1 AND COALESCE(d.profile_snapshot->'installer_ref'->>'name','')<>''
    AND CASE WHEN jsonb_typeof(d.profile_snapshot->'installer_ref'->'version')='number' THEN (d.profile_snapshot->'installer_ref'->>'version')::numeric>0 ELSE false END))
 )
), current_errors AS (
 SELECT cs.run_id, e.error_fingerprint
 FROM current_steps cs JOIN LATERAL (
  SELECT pe.error_fingerprint FROM provision_events pe
  WHERE pe.run_id=cs.run_id AND pe.step=cs.step AND COALESCE(pe.error_fingerprint,'')<>''
  AND (cs.last_started_at IS NULL OR pe.created_at>=cs.last_started_at)
  ORDER BY pe.created_at DESC,pe.id DESC LIMIT 1
 ) e ON true
 UNION
 SELECT mr.run_id,e.error_fingerprint FROM missing_installer_runs mr
 JOIN LATERAL (SELECT pe.error_fingerprint FROM provision_events pe WHERE pe.run_id=mr.run_id AND pe.step=mr.current_step AND pe.error_code='INSTALLER_NOT_CONFIGURED' ORDER BY pe.created_at DESC,pe.id DESC LIMIT 1) e ON true
), current_runs AS (
 SELECT run_id FROM current_steps UNION SELECT run_id FROM current_errors UNION SELECT run_id FROM missing_installer_runs
 UNION SELECT pr.id FROM provision_runs pr JOIN droplets dr ON dr.id=pr.droplet_id AND dr.account_id=pr.account_id
 WHERE pr.state='FAILED' AND dr.state<>'DELETED'
), event_groups AS (
 SELECT pe.error_fingerprint fingerprint,COALESCE(pe.error_class,'') error_class,COALESCE(pe.error_code,'') error_code,
 count(*) occurrences,count(DISTINCT pr.account_id) affected_accounts,
 array_agg(DISTINCT pr.droplet_id) droplets,
 count(DISTINCT pr.id) affected_runs,
 count(DISTINCT pr.id) FILTER(WHERE pr.state='COMPLETED') recovered_runs,
 count(DISTINCT pr.id) FILTER(WHERE pr.state<>'COMPLETED' AND dr.state='DELETED') retired_runs,
 count(DISTINCT pr.id) FILTER(WHERE EXISTS(SELECT 1 FROM current_errors ce WHERE ce.run_id=pr.id AND ce.error_fingerprint=pe.error_fingerprint)) open_runs,
 count(DISTINCT pr.id) FILTER(WHERE pr.state='WAITING_INSTALLER' AND EXISTS(SELECT 1 FROM current_errors ce WHERE ce.run_id=pr.id AND ce.error_fingerprint=pe.error_fingerprint)) waiting_runs,
 min(pe.created_at) first_seen,max(pe.created_at) last_seen,
 COALESCE((array_agg(pe.error_message ORDER BY pe.created_at DESC,pe.id DESC))[1],'') last_error,
 COALESCE((array_agg(pe.step ORDER BY pe.created_at DESC,pe.id DESC))[1],'') last_step,
 COALESCE((array_agg(COALESCE(pe.substep,'') ORDER BY pe.created_at DESC,pe.id DESC))[1],'') last_substep
 FROM provision_events pe JOIN provision_runs pr ON pr.id=pe.run_id
 JOIN droplets dr ON dr.id=pr.droplet_id AND dr.account_id=pr.account_id
 WHERE COALESCE(pe.error_fingerprint,'')<>''
 GROUP BY pe.error_fingerprint,pe.error_class,pe.error_code
), recent AS (
 SELECT g.fingerprint,g.error_class,g.error_code,g.occurrences,g.affected_accounts,g.affected_runs,g.recovered_runs,g.retired_runs,g.open_runs,g.waiting_runs,g.first_seen,g.last_seen,g.last_error,g.last_step,g.last_substep,
 (SELECT count(DISTINCT d.id) FROM deployments d WHERE d.droplet_id=ANY(g.droplets)) affected_deployments
 FROM event_groups g ORDER BY (g.open_runs>0) DESC,g.last_seen DESC,g.fingerprint LIMIT 200
)
SELECT json_build_object('records',COALESCE((SELECT json_agg(recent ORDER BY (open_runs>0) DESC,last_seen DESC,fingerprint) FROM recent),'[]'::json),
 'historical_families',(SELECT count(*) FROM event_groups),'current_failed_runs',(SELECT count(*) FROM current_runs),'observed_at',statement_timestamp())`

func (s *Server) provisionErrors(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	if err := s.DB.QueryRowContext(r.Context(), provisionErrorsSQL).Scan(&raw); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var result struct {
		Records            []map[string]any `json:"records"`
		HistoricalFamilies int              `json:"historical_families"`
		CurrentFailedRuns  int              `json:"current_failed_runs"`
		ObservedAt         string           `json:"observed_at"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	for _, row := range result.Records {
		if msg, ok := row["last_error"].(string); ok {
			row["last_error"] = redact.Text(msg)
		}
	}
	if r.URL.Query().Get("summary") == "1" {
		writeJSON(w, 200, result)
		return
	}
	writeJSON(w, 200, result.Records)
}
