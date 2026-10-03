package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/lib/pq"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) persistOutputSnapshot(ctx context.Context, panelID string, records []outputRecord) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE current_output_uris(uri text, visible_until timestamptz) ON COMMIT DROP`); err != nil {
		return
	}
	copyStmt, err := tx.PrepareContext(ctx, pq.CopyIn("current_output_uris", "uri", "visible_until"))
	if err != nil {
		return
	}
	for _, rec := range records {
		if !strings.HasPrefix(rec.URI, "vless://") {
			continue
		}
		if _, err = copyStmt.ExecContext(ctx, rec.URI, rec.VisibleUntil); err != nil {
			_ = copyStmt.Close()
			return
		}
	}
	if _, err = copyStmt.ExecContext(ctx); err != nil {
		_ = copyStmt.Close()
		return
	}
	if err = copyStmt.Close(); err != nil {
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO output_config_snapshots(panel_id,uri,visible_until)
SELECT $1,c.uri,CASE WHEN dr.expires_at IS NULL THEN c.visible_until WHEN c.visible_until IS NULL THEN dr.expires_at-interval '10 seconds' ELSE LEAST(c.visible_until,dr.expires_at-interval '10 seconds') END
FROM (SELECT DISTINCT ON(uri) uri,visible_until FROM current_output_uris ORDER BY uri) c
JOIN panel_instances p ON p.id=$1 JOIN droplets dr ON dr.id=p.droplet_id
ON CONFLICT(panel_id,uri) DO UPDATE SET last_seen_at=now(),visible_until=excluded.visible_until
WHERE output_config_snapshots.last_seen_at < now()-interval '10 seconds'
   OR output_config_snapshots.visible_until IS DISTINCT FROM excluded.visible_until`, panelID); err != nil {
		return
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM output_config_snapshots o WHERE o.panel_id=$1 AND NOT EXISTS (SELECT 1 FROM current_output_uris c WHERE c.uri=o.uri)`, panelID); err != nil {
		return
	}
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
	s.refreshOutputLive(r.Context())
	s.outputSnapshotResponse(w, r)
}
func (s *Server) outputSnapshotResponse(w http.ResponseWriter, r *http.Request) {
	where := ` WHERE o.last_seen_at>=now()-interval '15 seconds' AND p.enabled=true AND d.state='PANEL_COMPLETE' AND a.provider_state='ACTIVE' AND ` + outputDropletStatePredicate + ` AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds') AND (o.visible_until IS NULL OR o.visible_until>now()) `
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
