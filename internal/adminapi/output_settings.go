package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"io"
	"net/http"
)

type outputSettings struct {
	Revision int64 `json:"revision"`
	export.OutputOptions
}

func (s *Server) getOutputSettings(w http.ResponseWriter, r *http.Request) {
	var p outputSettings
	err := s.DB.QueryRowContext(r.Context(), `SELECT revision,include_xhttp_extra,include_finalmask FROM output_export_policy WHERE singleton`).Scan(&p.Revision, &p.IncludeXHTTPExtra, &p.IncludeFinalMask)
	if err != nil || p.Revision < 1 {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, p)
}
func (s *Server) putOutputSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision *int64 `json:"revision"`
		Extra    *bool  `json:"include_xhttp_extra"`
		Mask     *bool  `json:"include_finalmask"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF || req.Revision == nil || *req.Revision < 1 || req.Extra == nil || req.Mask == nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_output_settings"})
		return
	}
	var p outputSettings
	err := s.DB.QueryRowContext(r.Context(), `UPDATE output_export_policy SET revision=revision+1,include_xhttp_extra=$1,include_finalmask=$2,updated_at=now() WHERE singleton AND revision=$3 RETURNING revision,include_xhttp_extra,include_finalmask`, *req.Extra, *req.Mask, *req.Revision).Scan(&p.Revision, &p.IncludeXHTTPExtra, &p.IncludeFinalMask)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 409, map[string]string{"error": "output_settings_changed", "detail": "Reload output settings before saving."})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, p)
}
