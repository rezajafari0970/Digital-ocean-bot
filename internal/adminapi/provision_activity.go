package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
	"net/http"
)

func (s *Server) provisionActivity(w http.ResponseWriter, r *http.Request) {
	tx, err := s.DB.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	deploymentID := r.PathValue("id")
	var runID, state, step, lastErr string
	var attempt int
	var next any
	err = tx.QueryRowContext(r.Context(), `SELECT pr.id::text,pr.state,pr.current_step,pr.attempt,COALESCE(pr.last_error,''),pr.next_retry_at
FROM deployments d JOIN provision_runs pr ON pr.droplet_id=d.droplet_id AND pr.account_id=d.account_id
WHERE d.id=$1`, deploymentID).Scan(&runID, &state, &step, &attempt, &lastErr, &next)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// A known deployment can be waiting for provider confirmation before
			// SSH provisioning exists. Show that stage, not a misleading error.
			err = tx.QueryRowContext(r.Context(), `SELECT state,current_step,attempt,COALESCE(last_error,'') FROM deployments WHERE id=$1`, deploymentID).Scan(&state, &step, &attempt, &lastErr)
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, 404, map[string]string{"error": "deployment_not_found"})
			} else if err != nil {
				writeJSON(w, 500, errorBody())
			} else {
				writeJSON(w, 200, map[string]any{"deployment_id": deploymentID, "provisioning_started": false, "state": state, "current_step": step, "attempt": attempt, "last_error": redact.Text(lastErr), "steps": []any{}, "events": []any{}})
			}
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}
	rows, err := tx.QueryContext(r.Context(), `SELECT step,attempts,last_started_at,last_finished_at,COALESCE(last_error,''),next_retry_at,terminal FROM provision_step_attempts WHERE run_id=$1 ORDER BY step`, runID)
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
		if err := rows.Scan(&st, &n, &started, &finished, &le, &retry, &terminal); err != nil {
			writeJSON(w, 500, errorBody())
			return
		} else {
			attempts = append(attempts, map[string]any{"step": st, "attempts": n, "last_started_at": started, "last_finished_at": finished, "last_error": redact.Text(le), "next_retry_at": retry, "terminal": terminal})
		}
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	rows.Close()
	ev, err := tx.QueryContext(r.Context(), `SELECT step,COALESCE(substep,''),state,COALESCE(attempt,0),COALESCE(error_class,''),COALESCE(error_code,''),COALESCE(error_message,''),COALESCE(error_fingerprint,''),exit_code,COALESCE(signal,''),COALESCE(stdout_tail,''),COALESCE(stderr_tail,''),COALESCE(duration_ms,0),retryable,next_retry_at,metadata,created_at FROM provision_events WHERE run_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200`, runID)
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
		var meta json.RawMessage
		var created any
		if err := ev.Scan(&st, &sub, &statev, &a, &ec, &code, &msg, &fp, &exit, &signal, &stdout, &stderr, &dur, &retryable, &retryAt, &meta, &created); err != nil {
			writeJSON(w, 500, errorBody())
			return
		} else {
			events = append(events, map[string]any{"step": st, "substep": sub, "state": statev, "attempt": a, "error_class": ec, "error_code": code, "error": redact.Text(msg), "fingerprint": fp, "exit_code": exit, "signal": signal, "stdout_tail": redact.Text(stdout), "stderr_tail": redact.Text(stderr), "duration_ms": dur, "retryable": retryable, "next_retry_at": retryAt, "metadata": meta, "created_at": created})
		}
	}
	if ev.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	ev.Close()
	readiness := map[string]any{}
	var readinessID, rs, osid, osv, arch, pm, ph, ts string
	var cpu int
	var mem, disk int64
	var root, dns, https, reboot bool
	var checks json.RawMessage
	var created any
	readinessErr := tx.QueryRowContext(r.Context(), `SELECT id::text,status,os_id,os_version,architecture,cpu_count,memory_mb,disk_free_mb,is_root,package_manager,package_health,dns_ok,outbound_https_ok,time_sync,reboot_required,checks,created_at FROM server_readiness_snapshots WHERE run_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, runID).Scan(&readinessID, &rs, &osid, &osv, &arch, &cpu, &mem, &disk, &root, &pm, &ph, &dns, &https, &ts, &reboot, &checks, &created)
	if readinessErr != nil && !errors.Is(readinessErr, sql.ErrNoRows) {
		writeJSON(w, 500, errorBody())
		return
	}
	if readinessErr == nil {
		readiness = map[string]any{"status": rs, "os_id": osid, "os_version": osv, "architecture": arch, "cpu_count": cpu, "memory_mb": mem, "disk_free_mb": disk, "is_root": root, "package_manager": pm, "package_health": ph, "dns_ok": dns, "outbound_https_ok": https, "time_sync": ts, "reboot_required": reboot, "checks": checks, "created_at": created}
		ir, err := tx.QueryContext(r.Context(), `SELECT code,severity,action,remediation,state,detail FROM server_readiness_issues WHERE readiness_id=$1 ORDER BY created_at,id`, readinessID)
		issues := []map[string]any{}
		if err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if ir != nil {
			defer ir.Close()
			for ir.Next() {
				var c, se, a, rm, st, de string
				if err := ir.Scan(&c, &se, &a, &rm, &st, &de); err != nil {
					writeJSON(w, 500, errorBody())
					return
				} else {
					issues = append(issues, map[string]any{"code": c, "severity": se, "action": a, "remediation": rm, "state": st, "detail": redact.Text(de)})
				}
			}
		}
		if ir.Err() != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		ir.Close()
		readiness["issues"] = issues
	}
	installer := map[string]any{}
	installerHistory := []map[string]any{}
	if rows, err := tx.QueryContext(r.Context(), `SELECT i.name,i.version,ir.state,ir.manifest_sha256,ir.last_error,ir.generation,ir.created_at,ir.updated_at FROM installer_runs ir JOIN installers i ON i.id=ir.installer_id WHERE ir.provision_run_id=$1 ORDER BY ir.generation DESC`, runID); err != nil {
		writeJSON(w, 500, errorBody())
		return
	} else {
		defer rows.Close()
		for rows.Next() {
			var iname, istate, ihash, ierr string
			var iver, igen int
			var created, updated any
			if err := rows.Scan(&iname, &iver, &istate, &ihash, &ierr, &igen, &created, &updated); err != nil {
				writeJSON(w, 500, errorBody())
				return
			} else {
				x := map[string]any{"name": iname, "version": iver, "generation": igen, "state": istate, "sha256": ihash, "last_error": redact.Text(ierr), "created_at": created, "updated_at": updated}
				installerHistory = append(installerHistory, x)
				if len(installer) == 0 {
					installer = x
				}
			}
		}
		if rows.Err() != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		rows.Close()
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"run_id": runID, "state": state, "current_step": step, "attempt": attempt, "last_error": redact.Text(lastErr), "next_retry_at": next, "steps": attempts, "events": events, "readiness": readiness, "installer": installer, "installer_history": installerHistory})
}
