package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"io"
	"net/http"
)

func (s *Server) getConfigTransport(w http.ResponseWriter, r *http.Request) {
	p, e := transportprofile.Read(r.Context(), s.DB)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"policy": p, "preset": transportprofile.Preset(), "max_path_bytes": transportprofile.MaxPathBytes, "scope": "New physical listeners only. Existing listeners retain their saved transport and path. Direct and Residential clients on one listener share its path.", "finalmask_status": transportprofile.FinalMaskStatus, "compatibility": "XHTTP Reality stream-up uses native dialing and empty flow. WebView and Vision are not supported by this preset."})
}
func (s *Server) putConfigTransport(w http.ResponseWriter, r *http.Request) {
	var p transportprofile.Policy
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	dec.DisallowUnknownFields()
	if dec.Decode(&p) != nil || dec.Decode(new(any)) != io.EOF || p.Revision < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_transport_request"})
		return
	}
	if p.Config.Network == "" || p.Config.Validate(false) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_transport", "detail": "Use TCP or XHTTP stream-up with a domain Host and path size 256–4096. XHTTP requires empty Flow, disabled Browser Dialer and disabled FinalMask. Preset JSON is preserved unchanged."})
		return
	}
	raw, e := json.Marshal(p.Config)
	if e != nil {
		writeJSON(w, 400, errorBody())
		return
	}
	res, e := s.DB.ExecContext(r.Context(), `UPDATE reality_transport_policy SET config=$1,revision=revision+1,updated_at=now() WHERE singleton AND revision=$2`, raw, p.Revision)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, e := res.RowsAffected()
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if n != 1 {
		writeJSON(w, 409, map[string]string{"error": "transport_revision_changed", "detail": "Reload transport settings before saving."})
		return
	}
	s.getConfigTransport(w, r)
}
