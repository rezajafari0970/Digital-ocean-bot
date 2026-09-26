package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net/http"
	"time"
)

func (s *Server) listInstallScripts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,name,version,category,timeout_seconds,max_attempts,sha256,active,created_at FROM install_scripts ORDER BY name,version DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, cat, hash string
		var ver, timeout, max int
		var active bool
		var created time.Time
		if rows.Scan(&id, &name, &ver, &cat, &timeout, &max, &hash, &active, &created) == nil {
			out = append(out, map[string]any{"id": id, "name": name, "version": ver, "category": cat, "timeout_seconds": timeout, "max_attempts": max, "sha256": hash, "active": active, "created_at": created})
		}
	}
	writeJSON(w, 200, out)
}
func (s *Server) createInstallScript(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Name           string `json:"name"`
		Version        int    `json:"version"`
		Category       string `json:"category"`
		Precheck       string `json:"precheck"`
		Execute        string `json:"execute"`
		Verify         string `json:"verify"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		MaxAttempts    int    `json:"max_attempts"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&x); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_json"})
		return
	}
	step := provisioning.ScriptStep{Name: x.Name, Version: x.Version, Category: x.Category, Precheck: x.Precheck, Execute: x.Execute, Verify: x.Verify, Timeout: time.Duration(x.TimeoutSeconds) * time.Second, MaxAttempts: x.MaxAttempts}
	got, err := (provisioning.ScriptRegistry{DB: s.DB}).Create(r.Context(), step)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "script_registry_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"id": got.RegistryID, "name": got.Name, "version": got.Version, "sha256": got.SHA256})
}
