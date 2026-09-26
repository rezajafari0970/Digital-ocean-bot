package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net/http"
)

func (s *Server) listInstallers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,name,version,manifest,sha256,active,created_at FROM installers ORDER BY name,version DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, hash string
		var version int
		var raw json.RawMessage
		var active bool
		var created any
		if rows.Scan(&id, &name, &version, &raw, &hash, &active, &created) == nil {
			var m provisioning.InstallerManifest
			_ = json.Unmarshal(raw, &m)
			out = append(out, map[string]any{"id": id, "name": name, "version": version, "sha256": hash, "active": active, "created_at": created, "supported_os": m.SupportedOS, "supported_arch": m.SupportedArch, "min_memory_mb": m.MinMemoryMB, "min_disk_mb": m.MinDiskMB, "artifacts": len(m.Artifacts), "install_scripts": m.InstallScripts, "verify_scripts": m.VerifyScripts, "rollback_scripts": m.RollbackScripts, "services": m.Services})
		}
	}
	writeJSON(w, 200, out)
}
func (s *Server) createInstaller(w http.ResponseWriter, r *http.Request) {
	var m provisioning.InstallerManifest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_json"})
		return
	}
	reg := provisioning.InstallerRegistry{DB: s.DB, Scripts: provisioning.ScriptRegistry{DB: s.DB}}
	got, err := reg.Create(r.Context(), m)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "installer_registry_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"id": got.RegistryID, "name": m.Name, "version": m.Version, "sha256": got.SHA256})
}
