package adminapi

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
	"net/http"
	"time"
)

// Diagnostic activity is positive and bounded: terminal/unknown states and
// deployments whose linked resource was deleted are never counted as building.
// This predicate does not authorize worker mutations.
const activeBuildPredicate = `d.state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL','REGISTERING_CLIENTS','REGISTERING_TRAFFIC')
 AND NOT EXISTS(SELECT 1 FROM droplets gone WHERE gone.account_id=d.account_id AND gone.state='DELETED' AND (gone.id=d.droplet_id OR (d.droplet_id IS NULL AND NULLIF(d.provider_id,'')=gone.provider_resource_id)))`

const buildActivitySQL = `SELECT d.id::text,a.name,d.state,d.current_step,d.attempt,COALESCE(d.last_error,''),d.created_at,d.updated_at,
 COALESCE((SELECT de.message FROM deployment_events de WHERE de.deployment_id=d.id ORDER BY de.created_at DESC,de.id DESC LIMIT 1),''),
 (` + activeBuildPredicate + `) active,
 count(*) FILTER(WHERE ` + activeBuildPredicate + `) OVER(),
 count(*) FILTER(WHERE d.state='WAITING_INSTALLER' AND (` + activeBuildPredicate + `)) OVER(),
 COALESCE(op.error_code,''),COALESCE(op.error_message,''), EXISTS(SELECT 1 FROM droplets gone WHERE gone.account_id=d.account_id AND gone.state='DELETED' AND (gone.id=d.droplet_id OR (d.droplet_id IS NULL AND NULLIF(d.provider_id,'')=gone.provider_resource_id)))
 FROM deployments d JOIN accounts a ON a.id=d.account_id
 LEFT JOIN LATERAL(SELECT o.error_code,o.error_message FROM operations o WHERE d.droplet_id IS NULL AND COALESCE(d.provider_id,'')='' AND d.state NOT IN ('READY','PANEL_COMPLETE') AND o.account_id=d.account_id AND o.kind='CREATE_DROPLET' AND o.state<>'succeeded' AND (o.idempotency_key='deploy:'||d.id::text||':create' OR o.idempotency_key LIKE 'deploy:'||d.id::text||':create:region:%') ORDER BY o.updated_at DESC,o.id DESC LIMIT 1) op ON true
 ORDER BY d.updated_at DESC,d.id DESC LIMIT 100`

func (s *Server) buildActivity(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), buildActivitySQL)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	activeTotal, waitingTotal := 0, 0
	for rows.Next() {
		var id, name, state, step, lastErr, msg, operationCode, operationError string
		var attempt int
		var active, retired bool
		var created, updated time.Time
		if err = rows.Scan(&id, &name, &state, &step, &attempt, &lastErr, &created, &updated, &msg, &active, &activeTotal, &waitingTotal, &operationCode, &operationError, &retired); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		_, operationError = jobs.SafeDiagnostic(operationCode, operationError)
		out = append(out, map[string]any{"id": id, "account": name, "state": state, "step": step, "attempt": attempt, "error": redact.Text(lastErr), "message": redact.Text(msg), "retired": retired, "created_at": created, "updated_at": updated, "active": active, "operation_error": operationError})
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	// Preserve the original array contract for existing consumers.
	if r.URL.Query().Get("summary") == "1" {
		writeJSON(w, 200, map[string]any{"records": out, "active_builds": activeTotal, "waiting_builds": waitingTotal, "observed_at": time.Now().UTC()})
		return
	}
	writeJSON(w, 200, out)
}
