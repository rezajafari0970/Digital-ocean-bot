package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/clienttransport"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

var transportUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// UI offers current servers; saved profiles remain visible for rollback if retired.
func (s *Server) outputClientTransportStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r.Context())
	if !ok || !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "admin_required"})
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT p.id::text,p.base_url,COALESCE(t.preset,'off'),COALESCE(t.revision,0),
 p.enabled AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
 FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id LEFT JOIN output_client_transport_profiles t ON t.panel_id=p.id
 WHERE (p.enabled AND dr.state IN('READY','EXPIRING')) OR t.panel_id IS NOT NULL ORDER BY p.base_url,p.id`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	type item struct {
		PanelID   string `json:"panel_id"`
		Server    string `json:"server"`
		Preset    string `json:"preset"`
		Revision  int64  `json:"revision"`
		Available bool   `json:"available"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		var raw string
		if rows.Scan(&x.PanelID, &raw, &x.Preset, &x.Revision, &x.Available) != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		u, e := url.Parse(raw)
		if e == nil {
			x.Server = u.Host
		}
		out = append(out, x)
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"servers": out, "scope": "selected_existing_panel_only", "client_refresh_required": true})
}
func (s *Server) putOutputClientTransport(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r.Context())
	if !ok || !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "admin_required"})
		return
	}
	var req struct {
		Preset           string `json:"preset"`
		ExpectedRevision *int64 `json:"expected_revision"`
		OperationID      string `json:"operation_id"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(&struct{}{}) != io.EOF || !clienttransport.Valid(req.Preset) || req.ExpectedRevision == nil || *req.ExpectedRevision < 0 || !transportUUID.MatchString(req.OperationID) || !transportUUID.MatchString(r.PathValue("id")) {
		writeJSON(w, 400, map[string]string{"error": "invalid_client_transport_request"})
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var account string
	var available bool
	e = tx.QueryRowContext(ctx, `SELECT p.account_id::text,p.enabled AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds') FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id WHERE p.id=$1 FOR UPDATE OF p`, id).Scan(&account, &available)
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var oldPreset string
	var expected, result int64
	e = tx.QueryRowContext(ctx, `SELECT preset,expected_revision,result_revision FROM output_client_transport_operations WHERE panel_id=$1 AND operation_id=$2`, id, req.OperationID).Scan(&oldPreset, &expected, &result)
	if e == nil {
		if oldPreset != req.Preset || expected != *req.ExpectedRevision {
			writeJSON(w, 409, map[string]string{"error": "operation_id_reused"})
			return
		}
		writeJSON(w, 200, map[string]any{"preset": oldPreset, "revision": result, "replayed": true})
		return
	}
	if e != sql.ErrNoRows {
		writeJSON(w, 500, errorBody())
		return
	}
	var revision int64
	e = tx.QueryRowContext(ctx, `SELECT revision FROM output_client_transport_profiles WHERE panel_id=$1`, id).Scan(&revision)
	if e != nil && e != sql.ErrNoRows {
		writeJSON(w, 500, errorBody())
		return
	}
	if revision != *req.ExpectedRevision {
		writeJSON(w, 409, map[string]string{"error": "client_transport_revision_changed", "detail": "Settings changed. Refresh and retry."})
		return
	}
	if req.Preset != "off" {
		fresh := false
		rows, err := tx.QueryContext(ctx, `SELECT uri FROM output_config_snapshots WHERE panel_id=$1 AND last_seen_at>now()-interval '15 seconds' AND (visible_until IS NULL OR visible_until>now())`, id)
		if err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		valid := true
		for rows.Next() {
			var uri string
			if rows.Scan(&uri) != nil {
				valid = false
				break
			}
			if _, err = clienttransport.Apply(uri, req.Preset); err != nil {
				valid = false
				break
			}
			fresh = true
		}
		scanErr := rows.Err()
		rows.Close()
		if scanErr != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if !valid {
			writeJSON(w, 409, map[string]string{"error": "incompatible_client_output", "detail": "Existing output is not a compatible unmasked Reality configuration."})
			return
		}

		if !available || !fresh {
			writeJSON(w, 409, map[string]string{"error": "server_not_ready", "detail": "The selected server has no fresh Reality output."})
			return
		}
	}
	revision++
	_, e = tx.ExecContext(ctx, `INSERT INTO output_client_transport_profiles(panel_id,preset,revision) VALUES($1,$2,$3) ON CONFLICT(panel_id) DO UPDATE SET preset=excluded.preset,revision=excluded.revision,updated_at=now()`, id, req.Preset, revision)
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO output_client_transport_operations(panel_id,operation_id,expected_revision,preset,result_revision) VALUES($1,$2,$3,$4,$5)`, id, req.OperationID, *req.ExpectedRevision, req.Preset, revision)
	}
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO audit_events(account_id,actor,action,resource_type,resource_id,result,message,metadata) VALUES($1,$2,'output_client_transport_changed','panel',$3,'ok','Client export profile changed',jsonb_build_object('preset',$4::text,'revision',$5::bigint,'operation_id',$6::text))`, account, p.Username, id, req.Preset, revision, req.OperationID)
	}
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if tx.Commit() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"preset": req.Preset, "revision": revision, "client_refresh_required": true})
}
