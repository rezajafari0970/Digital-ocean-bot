package adminapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) persistOutputSnapshot(ctx context.Context, panelID string, records []outputRecord) error {
	return sanaei.WithConfigLock(ctx, s.DB, panelID, func(ctx context.Context) error {
		return s.persistOutputSnapshotOwned(ctx, panelID, records, nil)
	})
}
func (s *Server) persistOutputSnapshotOwned(ctx context.Context, panelID string, records []outputRecord, structures []outputStructure) (err error) {
	// Log the stage and SQLSTATE only: database error details can contain full
	// credential-bearing URIs. A failed transaction retains the previous snapshot.
	stage := "begin"
	defer func() {
		if err != nil {
			code := ""
			var pgErr *pq.Error
			if errors.As(err, &pgErr) {
				code = string(pgErr.Code)
			}
			log.Printf("output persist panel=%s stage=%s sqlstate=%s error_type=%T", panelID, stage, code, err)
		}
	}()
	return sanaei.WithConfigEvidenceTx(ctx, s.DB, panelID, func(tx *sql.Tx) (err error) {
		stage = "structural records"
		for _, record := range structures {
			if _, err = tx.ExecContext(ctx, `INSERT INTO inbound_structural_snapshots(panel_id,remote_id,port,payload,updated_at) VALUES($1,$2,$3,$4,now()) ON CONFLICT(panel_id,remote_id) DO UPDATE SET port=excluded.port,payload=excluded.payload,updated_at=now()`, panelID, record.ID, record.Port, record.Payload); err != nil {
				return err
			}
		}
		stage = "stage records"
		if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE current_output_uris(uri text, visible_until timestamptz,client_id text) ON COMMIT DROP`); err != nil {
			return err
		}
		copyStmt, err := tx.PrepareContext(ctx, pq.CopyIn("current_output_uris", "uri", "visible_until", "client_id"))
		if err != nil {
			return err
		}
		uniqueURIs := make(map[string]struct{}, len(records))
		for _, rec := range records {
			if !strings.HasPrefix(rec.URI, "vless://") {
				continue
			}
			uniqueURIs[rec.URI] = struct{}{}
			if _, err = copyStmt.ExecContext(ctx, rec.URI, rec.VisibleUntil, rec.ClientID); err != nil {
				_ = copyStmt.Close()
				return err
			}
		}
		if _, err = copyStmt.ExecContext(ctx); err != nil {
			_ = copyStmt.Close()
			return err
		}
		if err = copyStmt.Close(); err != nil {
			return err
		}
		stage = "upsert"
		result, err := tx.ExecContext(ctx, `INSERT INTO output_config_snapshots(panel_id,uri,visible_until,client_id)
SELECT $1,c.uri,CASE WHEN dr.expires_at IS NULL THEN c.visible_until WHEN c.visible_until IS NULL THEN dr.expires_at-interval '10 seconds' ELSE LEAST(c.visible_until,dr.expires_at-interval '10 seconds') END,c.client_id
FROM (SELECT DISTINCT ON(uri) uri,visible_until,client_id FROM current_output_uris ORDER BY uri) c
JOIN panel_instances p ON p.id=$1 JOIN droplets dr ON dr.id=p.droplet_id
ON CONFLICT(panel_id,(md5(uri))) DO UPDATE SET last_seen_at=now(),visible_until=excluded.visible_until,client_id=excluded.client_id
WHERE output_config_snapshots.uri=excluded.uri`, panelID)
		if err != nil {
			return err
		}
		// A digest is an index key, not URI identity. Never overwrite a different
		// URI even in the event of a hash collision.
		var affected int64
		affected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != int64(len(uniqueURIs)) {
			err = errors.New("output snapshot URI digest collision")
			return err
		}
		stage = "remove stale"
		if _, err = tx.ExecContext(ctx, `DELETE FROM output_config_snapshots o WHERE o.panel_id=$1 AND NOT EXISTS (SELECT 1 FROM current_output_uris c WHERE c.uri=o.uri)`, panelID); err != nil {
			return err
		}
		stage = "commit"
		return nil
	})
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
	s.outputSnapshotScoped(w, r, class, nil)
}
func (s *Server) outputSnapshotScoped(w http.ResponseWriter, r *http.Request, class string, scope *accountOutputScope) {
	if scope != nil && (scope.AccountID == "" || scope.Token == "" || (class != "DIRECT" && class != "RESIDENTIAL")) {
		http.Error(w, "invalid output scope", http.StatusBadRequest)
		return
	}
	where := ` WHERE o.last_seen_at>=now()-interval '15 seconds' AND p.enabled=true AND d.state='PANEL_COMPLETE' AND a.provider_state='ACTIVE' AND ` + outputDropletStatePredicate + ` AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds') AND (o.visible_until IS NULL OR o.visible_until>now()) `
	where += ` AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=p.id AND cj.state NOT IN('SUCCEEDED','CANCELLED')) `
	// A last verified admission closure stays withheld across stale/failed polls.
	// Only a valid open/disabled receipt proves recovery; a policy intent does not.
	where += serverProtectionOutputPredicate
	where += " AND NOT EXISTS(SELECT 1 FROM reality_transport_apply_jobs ta WHERE ta.panel_id=p.id AND ta.state IN('RUNNING','BLOCKED')) "
	args := []any{}
	// Class publication requires a fresh running-core proof of the current revision.
	proof := ` rs.state='APPLIED' AND rs.revision=rc.revision AND cr.revision=rc.revision AND rs.verified_at>now()-interval '60 seconds' `
	scoped := ` rc.enabled AND (rc.fleet OR p.id=ANY(rc.panel_ids)) `
	if class != "ALL" {
		args = append(args, class)
		where += " AND (" + scoped + ") AND (" + proof + ") AND cr.effective_class=$1 AND cr.route_class=$1 "
		// Profile links publish only clients whose immutable ownership carries
		// that profile. Preserved manual/default clients retain their own limits.
		where += ` AND (NOT EXISTS(SELECT 1 FROM reality_config_profiles) OR EXISTS(
         SELECT 1 FROM bulk_user_ownership own JOIN bulk_user_generations gen ON gen.id=own.generation_id
         JOIN reality_config_profiles profile ON profile.route_class=own.route_class
         WHERE gen.panel_id=p.id AND own.client_id=o.client_id AND own.email=cr.email
         AND own.state='ACTIVE' AND own.route_class=$1)) `
	} else {
		where += " AND (NOT (" + scoped + ") OR ((" + proof + ") AND cr.effective_class IN ('DIRECT','RESIDENTIAL'))) "
	}
	if scope != nil {
		args = append(args, scope.AccountID, scope.Token)
		// Revalidate capability and ownership in the same statement snapshot as output.
		where += ` AND dr.account_id=$2 AND p.account_id=dr.account_id AND d.account_id=dr.account_id
   AND a.deleted_at IS NULL AND EXISTS(SELECT 1 FROM account_output_share_tokens ast
    WHERE ast.token=$3 AND ast.account_id=dr.account_id AND ast.route_class=$1 AND ast.revoked_at IS NULL) `
	}
	// A failed health check hides residential links immediately, ahead of reconciliation.
	where += ` AND (cr.effective_class IS DISTINCT FROM 'RESIDENTIAL' OR
 (rs.relay_mode AND (` + residentialsync.RelayOutputHealthySQL + `)) OR
 (NOT rs.relay_mode AND EXISTS(
 SELECT 1 FROM residential_proxies rp WHERE (rs.pool_enabled OR rp.proxy_id=rs.selected_proxy_id) AND rp.enabled AND rp.status='healthy' AND rp.last_success_at>now()-interval '3 minutes'))) `
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
	columns := `o.uri,NULL::jsonb AS ports`
	if class != "ALL" {
		columns = `o.uri,(SELECT ports FROM reality_config_profiles WHERE route_class=$1) AS ports`
	}
	q := `SELECT ` + columns + ` FROM output_config_snapshots o JOIN panel_instances p ON p.id=o.panel_id JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id CROSS JOIN residential_routing_control rc LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id LEFT JOIN panel_client_routes cr ON cr.panel_id=p.id AND cr.client_id=o.client_id ` + where + ` ORDER BY random()`
	// The sentinel retains a row even for empty output or a missing policy.
	// Both policy and eligible URIs use this one statement snapshot.
	q = `SELECT ep.revision,ep.include_xhttp_extra,ep.include_finalmask,published.uri,published.ports
 FROM (SELECT true AS singleton) anchor
 LEFT JOIN output_export_policy ep ON ep.singleton=anchor.singleton
 LEFT JOIN LATERAL (` + q + `) published ON true`
	rows, e := s.DB.QueryContext(r.Context(), q, args...)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out strings.Builder
	for rows.Next() {
		var raw sql.NullString
		var revision sql.NullInt64
		var extra, mask sql.NullBool
		var ports []byte
		if err := rows.Scan(&revision, &extra, &mask, &raw, &ports); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if !revision.Valid || revision.Int64 < 1 || !extra.Valid || !mask.Valid {
			writeJSON(w, 500, errorBody())
			return
		}
		if !raw.Valid {
			continue
		}
		if outputProfilePortAllowed(raw.String, ports) {
			u, err := export.ApplyOutputOptions(raw.String, export.OutputOptions{IncludeXHTTPExtra: extra.Bool, IncludeFinalMask: mask.Bool})
			if err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
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

// URI and profile ports come from the same database statement snapshot.
func outputProfilePortAllowed(uri string, raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var ports []int
	if json.Unmarshal(raw, &ports) != nil {
		return false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "vless" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return false
	}
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}
