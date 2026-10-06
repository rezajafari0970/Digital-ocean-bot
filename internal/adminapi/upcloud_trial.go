package adminapi

import (
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
)

func (s *Server) enableUpCloudTrial(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x struct {
		Version int64 `json:"version"`
		Accept  bool  `json:"restricted_egress_accepted"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&x) != nil || x.Version <= 0 || !x.Accept {
		writeJSON(w, 400, map[string]string{"error": "confirm_restricted_trial_deployment"})
		return
	}
	if err := capacity.EnableUpCloudTrial(r.Context(), s.DB, r.PathValue("id"), x.Version, p.Username); err != nil {
		var e *capacity.TrialModeError
		if errors.As(err, &e) {
			status := 409
			if e.Code == "not_found" {
				status = 404
			}
			writeJSON(w, status, map[string]string{"error": e.Code, "detail": e.Detail})
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}
	writeJSON(w, 200, map[string]any{"status": "trial_compatible_enabled", "panel_port": providers.UpCloudTrialPanelPort, "detail": "Existing scheduling and quota gates apply. Restricted destinations remain blocked; this does not verify residential connectivity or remove the trial firewall."})
}
