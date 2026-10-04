package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/lib/pq"
	"io"
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
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE current_output_uris(uri text, visible_until timestamptz,client_id text) ON COMMIT DROP`); err != nil {
		return
	}
	copyStmt, err := tx.PrepareContext(ctx, pq.CopyIn("current_output_uris", "uri", "visible_until", "client_id"))
	if err != nil {
		return
	}
	for _, rec := range records {
		if !strings.HasPrefix(rec.URI, "vless://") {
			continue
		}
		if _, err = copyStmt.ExecContext(ctx, rec.URI, rec.VisibleUntil, rec.ClientID); err != nil {
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
	if _, err = tx.ExecContext(ctx, `INSERT INTO output_config_snapshots(panel_id,uri,visible_until,client_id)
SELECT $1,c.uri,CASE WHEN dr.expires_at IS NULL THEN c.visible_until WHEN c.visible_until IS NULL THEN dr.expires_at-interval '10 seconds' ELSE LEAST(c.visible_until,dr.expires_at-interval '10 seconds') END,c.client_id
FROM (SELECT DISTINCT ON(uri) uri,visible_until,client_id FROM current_output_uris ORDER BY uri) c
JOIN panel_instances p ON p.id=$1 JOIN droplets dr ON dr.id=p.droplet_id
ON CONFLICT(panel_id,uri) DO UPDATE SET last_seen_at=now(),visible_until=excluded.visible_until,client_id=excluded.client_id`, panelID); err != nil {
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
	var body struct {
		RouteClass string `json:"route_class"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_route_class"})
		return
	}
	if body.RouteClass == "" {
		body.RouteClass = "ALL"
	}
	if !validOutputClass(body.RouteClass) {
		writeJSON(w, 400, map[string]string{"error": "invalid_route_class"})
		return
	}
	token := newShareToken()
	if token == "" {
		writeJSON(w, 500, errorBody())
		return
	}
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO output_share_tokens(token,route_class) VALUES($1,$2)`, token, body.RouteClass); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]string{"token": token, "url": "/share/output/" + token})
}
func (s *Server) sharedOutput(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var class string
	if s.DB.QueryRowContext(r.Context(), `SELECT route_class FROM output_share_tokens WHERE token=$1 AND revoked_at IS NULL`, token).Scan(&class) != nil {
		http.NotFound(w, r)
		return
	}
	// The token, not a query parameter, is the authority for its route class.
	if requested := r.URL.Query().Get("route_class"); requested != "" && requested != class {
		http.Error(w, "share route class is fixed", 400)
		return
	}
	if r.URL.Query().Get("view") == "1" {
		outputBrowserView(w)
		return
	}
	s.outputSnapshotClass(w, r, class)
}
func validOutputClass(c string) bool { return c == "ALL" || c == "RESIDENTIAL" || c == "DIRECT" }
func (s *Server) outputSnapshotResponse(w http.ResponseWriter, r *http.Request) {
	class := r.URL.Query().Get("route_class")
	if class == "" {
		class = "ALL"
	}
	if !validOutputClass(class) {
		http.Error(w, "invalid route class", 400)
		return
	}
	s.outputSnapshotClass(w, r, class)
}
func (s *Server) outputSnapshotClass(w http.ResponseWriter, r *http.Request, class string) {
	where := ` WHERE o.last_seen_at>=now()-interval '15 seconds' AND p.enabled=true AND d.state='PANEL_COMPLETE' AND a.provider_state='ACTIVE' AND ` + outputDropletStatePredicate + ` AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds') AND (o.visible_until IS NULL OR o.visible_until>now()) `
	where += ` AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=p.id AND cj.state NOT IN('SUCCEEDED','CANCELLED')) `
	args := []any{}
	// Class publication requires a fresh running-core proof of the current revision.
	proof := ` rs.state='APPLIED' AND rs.revision=rc.revision AND cr.revision=rc.revision AND rs.verified_at>now()-interval '60 seconds' `
	scoped := ` rc.enabled AND (rc.fleet OR p.id=ANY(rc.panel_ids)) `
	if class != "ALL" {
		args = append(args, class)
		where += " AND (" + scoped + ") AND (" + proof + ") AND cr.effective_class=$1 AND cr.route_class=$1 "
	} else {
		where += " AND (NOT (" + scoped + ") OR ((" + proof + ") AND cr.effective_class IN ('DIRECT','RESIDENTIAL'))) "
	}
	// A failed health check hides residential links immediately, ahead of reconciliation.
	where += ` AND (cr.effective_class IS DISTINCT FROM 'RESIDENTIAL' OR EXISTS(
 SELECT 1 FROM residential_proxies rp WHERE rp.proxy_id=rs.selected_proxy_id AND rp.enabled AND rp.status='healthy' AND rp.last_success_at>now()-interval '3 minutes')) `
	if raw := r.URL.Query().Get("expires_within_minutes"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 1440 {
			http.Error(w, "invalid expiry window", 400)
			return
		}
		args = append(args, n)
		where += ` AND dr.expires_at>now() AND dr.expires_at<=now()+($` + strconv.Itoa(len(args)) + `*interval '1 minute') `
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
	q := `SELECT o.uri FROM output_config_snapshots o JOIN panel_instances p ON p.id=o.panel_id JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id CROSS JOIN residential_routing_control rc LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id LEFT JOIN panel_client_routes cr ON cr.panel_id=p.id AND cr.client_id=o.client_id ` + where + ` ORDER BY random()`
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
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(out.String()))
}
