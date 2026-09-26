package adminapi

import "net/http"

func (s *Server) provisionErrors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `
SELECT pe.error_fingerprint,COALESCE(pe.error_class,''),COALESCE(pe.error_code,''),
 count(*) AS occurrences,count(DISTINCT d.id) AS deployments,count(DISTINCT d.account_id) AS accounts,
 min(pe.created_at),max(pe.created_at),
 COALESCE((array_agg(pe.error_message ORDER BY pe.created_at DESC))[1],''),
 COALESCE((array_agg(pe.step ORDER BY pe.created_at DESC))[1],''),
 COALESCE((array_agg(COALESCE(pe.substep,'') ORDER BY pe.created_at DESC))[1],'')
FROM provision_events pe
JOIN provision_runs pr ON pr.id=pe.run_id
JOIN droplets dr ON dr.id=pr.droplet_id
JOIN deployments d ON d.droplet_id=dr.id AND d.account_id=pr.account_id
WHERE pe.error_fingerprint IS NOT NULL AND pe.error_fingerprint<>''
GROUP BY pe.error_fingerprint,pe.error_class,pe.error_code
ORDER BY max(pe.created_at) DESC LIMIT 200`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var fp, class, code, msg, step, sub string
		var occ, deps, accounts int
		var first, last any
		if rows.Scan(&fp, &class, &code, &occ, &deps, &accounts, &first, &last, &msg, &step, &sub) == nil {
			out = append(out, map[string]any{"fingerprint": fp, "error_class": class, "error_code": code, "occurrences": occ, "affected_deployments": deps, "affected_accounts": accounts, "first_seen": first, "last_seen": last, "last_error": msg, "last_step": step, "last_substep": sub})
		}
	}
	writeJSON(w, 200, out)
}
