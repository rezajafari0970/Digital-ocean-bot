package adminapi

import (
	"encoding/json"
	"net/http"
)

func (s *Server) selectDeploymentInstaller(w http.ResponseWriter, r *http.Request) {
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
	var ok bool
	if err := s.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM installers WHERE name=$1 AND version=$2 AND active=true)`, x.Name, x.Version).Scan(&ok); err != nil || !ok {
		writeJSON(w, 404, map[string]string{"error": "installer_not_found"})
		return
	}
	res, err := s.DB.ExecContext(r.Context(), `INSERT INTO deployment_installer_selections(deployment_id,installer_name,installer_version,source) SELECT id,$2,$3,'operator' FROM deployments WHERE id=$1 AND state='WAITING_INSTALLER' ON CONFLICT(deployment_id) DO NOTHING`, id, x.Name, x.Version)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "installer_selection_failed"})
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		writeJSON(w, 409, map[string]string{"error": "deployment_not_waiting_or_already_selected"})
		return
	}
	writeJSON(w, 201, map[string]any{"deployment_id": id, "installer": x.Name, "version": x.Version})
}
