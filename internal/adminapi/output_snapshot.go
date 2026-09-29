package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) persistOutputSnapshot(ctx context.Context, panelID, value string) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	for _, uri := range strings.Fields(value) {
		if !strings.HasPrefix(uri, "vless://") {
			continue
		}
		_, _ = tx.ExecContext(ctx, `INSERT INTO output_config_snapshots(panel_id,uri) VALUES($1,$2) ON CONFLICT(panel_id,uri) DO UPDATE SET last_seen_at=now()`, panelID, uri)
	}
	_, _ = tx.ExecContext(ctx, `DELETE FROM output_config_snapshots WHERE panel_id=$1 AND last_seen_at < now()-interval '5 minutes'`, panelID)
	_ = tx.Commit()
}
func newShareToken() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func (s *Server) createOutputShare(w http.ResponseWriter, r *http.Request) {
	token := newShareToken()
	if token == "" {
		writeJSON(w, 500, errorBody())
		return
	}
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO output_share_tokens(token) VALUES($1)`, token); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]string{"token": token, "url": "/share/output/" + token})
}
func (s *Server) sharedOutput(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var ok bool
	if s.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM output_share_tokens WHERE token=$1 AND revoked_at IS NULL)`, token).Scan(&ok) != nil || !ok {
		http.NotFound(w, r)
		return
	}
	s.outputSnapshotResponse(w, r)
}
func (s *Server) outputSnapshotResponse(w http.ResponseWriter, r *http.Request) {
	where := ` WHERE p.enabled=true AND d.state='PANEL_COMPLETE' AND a.provider_state<>'LOCKED' AND dr.state IN ('READY','EXPIRING','RETIRING') `
	args := []any{}
	if raw := r.URL.Query().Get("expires_within_minutes"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 1440 {
			http.Error(w, "invalid expiry window", 400)
			return
		}
		args = append(args, n)
		where += ` AND dr.expires_at>now() AND dr.expires_at<=now()+($1*interval '1 minute') `
	}
	if raw := r.URL.Query().Get("created_within_minutes"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 1440 {
			http.Error(w, "invalid created window", 400)
			return
		}
		args = append(args, n)
		where += ` AND o.first_seen_at>=now()-($` + strconv.Itoa(len(args)) + `*interval '1 minute') `
	}
	q := `SELECT o.uri FROM output_config_snapshots o JOIN panel_instances p ON p.id=o.panel_id JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id ` + where + ` ORDER BY random()`
	rows, e := s.DB.QueryContext(r.Context(), q, args...)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out strings.Builder
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			out.WriteString(u)
			out.WriteByte('\n')
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(out.String()))
}
