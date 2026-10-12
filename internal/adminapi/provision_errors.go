package adminapi

import "net/http"

func (s *Server) provisionErrors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `
WITH event_groups AS (
SELECT pe.error_fingerprint,COALESCE(pe.error_class,'') error_class,COALESCE(pe.error_code,'') error_code,
 count(*) AS occurrences,count(DISTINCT pr.account_id) AS accounts,
 array_agg(DISTINCT pr.droplet_id) droplets,
 count(DISTINCT pr.id) FILTER(WHERE pr.state<>'COMPLETED' AND pr.current_step=pe.step AND dr.state<>'DELETED' AND EXISTS(SELECT 1 FROM provision_step_attempts st WHERE st.run_id=pr.id AND st.step=pe.step AND COALESCE(st.last_error,'')<>'')) open_runs,
 count(DISTINCT pr.id) FILTER(WHERE pr.state='WAITING_INSTALLER' AND pr.current_step=pe.step AND dr.state<>'DELETED') waiting_runs,
 min(pe.created_at) first_seen,max(pe.created_at) last_seen,
 COALESCE((array_agg(pe.error_message ORDER BY pe.created_at DESC,pe.id DESC))[1],'') message,
 COALESCE((array_agg(pe.step ORDER BY pe.created_at DESC,pe.id DESC))[1],'') step,
 COALESCE((array_agg(COALESCE(pe.substep,'') ORDER BY pe.created_at DESC,pe.id DESC))[1],'') substep
FROM provision_events pe
JOIN provision_runs pr ON pr.id=pe.run_id
JOIN droplets dr ON dr.id=pr.droplet_id AND dr.account_id=pr.account_id
WHERE pe.error_fingerprint IS NOT NULL AND pe.error_fingerprint<>''
GROUP BY pe.error_fingerprint,pe.error_class,pe.error_code
ORDER BY max(pe.created_at) DESC LIMIT 200
)
SELECT g.error_fingerprint,g.error_class,g.error_code,g.occurrences,
 (SELECT count(DISTINCT d.id) FROM deployments d WHERE d.droplet_id=ANY(g.droplets)),
 g.accounts,g.first_seen,g.last_seen,g.message,g.step,g.substep,g.open_runs,g.waiting_runs
FROM event_groups g ORDER BY g.last_seen DESC,g.error_fingerprint`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var fp, class, code, msg, step, sub string
		var occ, deps, accounts, openRuns, waitingRuns int
		var first, last any
		if err = rows.Scan(&fp, &class, &code, &occ, &deps, &accounts, &first, &last, &msg, &step, &sub, &openRuns, &waitingRuns); err != nil {
			writeJSON(w, 500, errorBody())
			return
		} else {
			out = append(out, map[string]any{"fingerprint": fp, "error_class": class, "error_code": code, "occurrences": occ, "affected_deployments": deps, "affected_accounts": accounts, "first_seen": first, "last_seen": last, "last_error": msg, "last_step": step, "last_substep": sub, "open_runs": openRuns, "waiting_runs": waitingRuns})
		}
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, out)
}
