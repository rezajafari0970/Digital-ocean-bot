package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func (s *Server) rearmDeploymentInstaller(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var x struct {
		Name    string `json:"name"`
		Version int    `json:"version"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&x) != nil || x.Name == "" || x.Version < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_installer_ref"})
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), &sql.TxOptions{})
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var state string
	var generation int
	if err = tx.QueryRowContext(r.Context(), `SELECT state,installer_generation FROM deployments WHERE id=$1 FOR UPDATE`, id).Scan(&state, &generation); err != nil {
		writeJSON(w, 404, map[string]string{"error": "deployment_not_found"})
		return
	}
	if state != "INSTALL_FAILED" && state != "INSTALL_ROLLED_BACK" {
		writeJSON(w, 409, map[string]string{"error": "deployment_not_rearmable", "state": state})
		return
	}
	var active bool
	if err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM installers WHERE name=$1 AND version=$2 AND active=true)`, x.Name, x.Version).Scan(&active); err != nil || !active {
		writeJSON(w, 404, map[string]string{"error": "installer_not_found"})
		return
	}
	next := generation + 1
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO deployment_installer_selections(deployment_id,generation,installer_name,installer_version,source) VALUES($1,$2,$3,$4,'operator-rearm')`, id, next, x.Name, x.Version); err != nil {
		writeJSON(w, 409, map[string]string{"error": "installer_rearm_conflict"})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE deployments SET installer_generation=$2,state='WAITING_INSTALLER',current_step='installer_rearmed',last_error='',updated_at=now() WHERE id=$1`, id, next); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM worker_item_failures WHERE kind='deployment' AND item_id=$1`, id)
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 201, map[string]any{"deployment_id": id, "generation": next, "installer": x.Name, "version": x.Version, "state": "WAITING_INSTALLER"})
}
