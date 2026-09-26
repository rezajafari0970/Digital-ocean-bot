package adminapi

import (
	"encoding/json"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func (s *Server) createSanaeiInstaller(w http.ResponseWriter, r *http.Request) {
	var d sanaei.InstallerDefinition
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_json"})
		return
	}
	got, err := sanaei.RegisterInstaller(r.Context(), s.DB, d)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "sanaei_installer_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"id": got.RegistryID, "name": got.Manifest.Name, "version": got.Manifest.Version, "sha256": got.SHA256, "release": d.Release})
}
func (s *Server) previewSanaeiInstaller(w http.ResponseWriter, r *http.Request) {
	var d sanaei.InstallerDefinition
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_json"})
		return
	}
	m, scripts, err := d.Build()
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_sanaei_installer", "detail": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"manifest": m, "manifest_sha256": provisioning.InstallerHash(m), "scripts": scripts})
}
