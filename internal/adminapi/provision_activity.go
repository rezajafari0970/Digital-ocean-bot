package adminapi

import (
	"net/http"
)

func (s *Server) provisionActivity(w http.ResponseWriter, r *http.Request) {
	deploymentID := r.PathValue("id")
	var runID, state, step, lastErr string
	var attempt int
	var next any
	err := s.DB.QueryRowContext(r.Context(), `SELECT pr.id::text,pr.state,pr.current_step,pr.attempt,COALESCE(pr.last_error,''),pr.next_retry_at
FROM deployments d JOIN provision_runs pr ON pr.droplet_id=d.droplet_id AND pr.account_id=d.account_id
WHERE d.id=$1`, deploymentID).Scan(&runID, &state, &step, &attempt, &lastErr, &next)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provision_run_not_found"})
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT step,attempts,last_started_at,last_finished_at,COALESCE(last_error,''),next_retry_at,terminal FROM provision_step_attempts WHERE run_id=$1 ORDER BY step`, runID)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	attempts := []map[string]any{}
	for rows.Next() {
		var st, le string
		var n int
		var started, finished, retry any
		var terminal bool
		if rows.Scan(&st, &n, &started, &finished, &le, &retry, &terminal) == nil {
			attempts = append(attempts, map[string]any{"step": st, "attempts": n, "last_started_at": started, "last_finished_at": finished, "last_error": le, "next_retry_at": retry, "terminal": terminal})
		}
	}
	ev, err := s.DB.QueryContext(r.Context(), `SELECT step,COALESCE(substep,''),state,COALESCE(attempt,0),COALESCE(error_class,''),COALESCE(error_code,''),COALESCE(error_message,''),COALESCE(error_fingerprint,''),exit_code,COALESCE(signal,''),COALESCE(stdout_tail,''),COALESCE(stderr_tail,''),COALESCE(duration_ms,0),retryable,next_retry_at,metadata,created_at FROM provision_events WHERE run_id=$1 ORDER BY created_at DESC LIMIT 200`, runID)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer ev.Close()
	events := []map[string]any{}
	for ev.Next() {
		var st, sub, statev, ec, code, msg, fp, signal, stdout, stderr string
		var a, dur int
		var exit, retryAt any
		var retryable *bool
		var meta any
		var created any
		if ev.Scan(&st, &sub, &statev, &a, &ec, &code, &msg, &fp, &exit, &signal, &stdout, &stderr, &dur, &retryable, &retryAt, &meta, &created) == nil {
			events = append(events, map[string]any{"step": st, "substep": sub, "state": statev, "attempt": a, "error_class": ec, "error_code": code, "error": msg, "fingerprint": fp, "exit_code": exit, "signal": signal, "stdout_tail": stdout, "stderr_tail": stderr, "duration_ms": dur, "retryable": retryable, "next_retry_at": retryAt, "metadata": meta, "created_at": created})
		}
	}
	readiness := map[string]any{}
	var rs, osid, osv, arch, pm, ph, ts string
	var cpu int
	var mem, disk int64
	var root, dns, https, reboot bool
	var checks, created any
	if s.DB.QueryRowContext(r.Context(), `SELECT status,os_id,os_version,architecture,cpu_count,memory_mb,disk_free_mb,is_root,package_manager,package_health,dns_ok,outbound_https_ok,time_sync,reboot_required,checks,created_at FROM server_readiness_snapshots WHERE run_id=$1 ORDER BY created_at DESC LIMIT 1`, runID).Scan(&rs, &osid, &osv, &arch, &cpu, &mem, &disk, &root, &pm, &ph, &dns, &https, &ts, &reboot, &checks, &created) == nil {
		readiness = map[string]any{"status": rs, "os_id": osid, "os_version": osv, "architecture": arch, "cpu_count": cpu, "memory_mb": mem, "disk_free_mb": disk, "is_root": root, "package_manager": pm, "package_health": ph, "dns_ok": dns, "outbound_https_ok": https, "time_sync": ts, "reboot_required": reboot, "checks": checks, "created_at": created}
	}
	writeJSON(w, 200, map[string]any{"run_id": runID, "state": state, "current_step": step, "attempt": attempt, "last_error": lastErr, "next_retry_at": next, "steps": attempts, "events": events, "readiness": readiness})
}
