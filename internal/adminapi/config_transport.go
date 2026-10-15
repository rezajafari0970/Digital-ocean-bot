package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportapply"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"io"
	"net/http"
	"regexp"
)

func (s *Server) getConfigTransport(w http.ResponseWriter, r *http.Request) {
	p, e := transportprofile.Read(r.Context(), s.DB)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	jobs, e := transportapply.List(r.Context(), s.DB)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"policy": p, "preset": transportprofile.Recommended(), "max_path_bytes": transportprofile.MaxPathBytes,
		"scope":            "Saved defaults apply to new listeners. Select Apply to existing to update managed listeners in sequence. Existing client IDs and limits are preserved; clients must refresh subscriptions.",
		"finalmask_status": transportprofile.FinalMaskStatus, "applications": jobs,
		"browser_dialer_supported": false, "compatibility": "Browser Dialer is not available for these REALITY profiles and is not exported as an enabled client setting. Vision-udp443 uses VLESS Encryption on XHTTP."})
}

type transportRequest struct {
	transportprofile.Policy
	ApplyExisting bool `json:"apply_existing"`
}

func (s *Server) putConfigTransport(w http.ResponseWriter, r *http.Request) {
	var p transportRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	dec.DisallowUnknownFields()
	if dec.Decode(&p) != nil || dec.Decode(new(any)) != io.EOF || p.Revision < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_transport_request"})
		return
	}
	if p.Config.Network == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_transport", "detail": "Select an explicit network."})
		return
	}
	if e := p.Config.Validate(false); e != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_transport", "detail": e.Error()})
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	raw, _ := json.Marshal(p.Config)
	res, e := tx.ExecContext(r.Context(), "UPDATE reality_transport_policy SET config=$1,revision=revision+1,updated_at=now() WHERE singleton AND revision=$2", raw, p.Revision)
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
	if p.ApplyExisting {
		p.Revision++
		if _, e = transportapply.Queue(r.Context(), tx, p.Policy); e != nil {
			transportApplyError(w, e)
			return
		}
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	s.getConfigTransport(w, r)
}
func transportApplyError(w http.ResponseWriter, e error) {
	if errors.Is(e, transportapply.ErrBusy) || errors.Is(e, transportapply.ErrConflict) || errors.Is(e, sql.ErrNoRows) {
		writeJSON(w, 409, map[string]string{"error": "transport_apply_conflict", "detail": "An application is pending or the listener has changed. Check application status."})
		return
	}
	writeJSON(w, 500, errorBody())
}
func (s *Server) applyConfigTransport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision int64    `json:"revision"`
		Panels   []string `json:"panels,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF || req.Revision < 1 {
		writeJSON(w, 400, errorBody())
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	p, e := readLockedPolicy(r.Context(), tx)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if p.Revision != req.Revision {
		writeJSON(w, 409, map[string]string{"error": "transport_revision_changed"})
		return
	}
	if _, e = transportapply.Queue(r.Context(), tx, p, req.Panels...); e != nil {
		transportApplyError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	s.getConfigTransport(w, r)
}
func readLockedPolicy(ctx context.Context, tx *sql.Tx) (transportprofile.Policy, error) {
	var p transportprofile.Policy
	var raw []byte
	e := tx.QueryRowContext(ctx, "SELECT revision,config FROM reality_transport_policy WHERE singleton FOR UPDATE").Scan(&p.Revision, &raw)
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(raw, &p.Config)
	return p, e
}
func (s *Server) rollbackConfigTransport(w http.ResponseWriter, r *http.Request) {
	if !regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$").MatchString(r.PathValue("id")) {
		writeJSON(w, 400, errorBody())
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if e = transportapply.Rollback(r.Context(), tx, r.PathValue("id")); e != nil {
		transportApplyError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		transportApplyError(w, e)
		return
	}
	s.getConfigTransport(w, r)
}

func (s *Server) retryConfigTransport(w http.ResponseWriter, r *http.Request) {
	if !regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$").MatchString(r.PathValue("id")) {
		writeJSON(w, 400, errorBody())
		return
	}
	res, e := s.DB.ExecContext(r.Context(), "UPDATE reality_transport_apply_jobs SET state=CASE WHEN before_secret_ref='' AND source_config=before_config THEN 'PENDING' ELSE 'RUNNING' END,phase='RECHECK_REQUESTED',attempts=0,next_retry_at=now(),last_error='',updated_at=now() WHERE id=$1 AND state='BLOCKED'", r.PathValue("id"))
	if e != nil {
		transportApplyError(w, e)
		return
	}
	n, e := res.RowsAffected()
	if e != nil || n != 1 {
		transportApplyError(w, transportapply.ErrConflict)
		return
	}
	s.getConfigTransport(w, r)
}
