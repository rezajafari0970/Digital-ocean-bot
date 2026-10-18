package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/lib/pq"
	"io"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
)

// Counts come from one repeatable-read snapshot. Verified runtime and published
// client counts are distinct; stale observations never count as ready output.
func (s *Server) getConfigProfiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var unified bool
	var reset sql.NullString
	var observed time.Time
	if e = tx.QueryRowContext(ctx, "SELECT unified,reset_job_id::text,now() FROM config_control_authority WHERE singleton").Scan(&unified, &reset, &observed); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	rows, e := tx.QueryContext(ctx, "SELECT id::text FROM config_driver_profiles WHERE deleted_at IS NULL ORDER BY created_at,id")
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	profiles := []map[string]any{}
	where, _ := outputPublicationFilter("ALL")
	// Evaluate the expensive publication fences once per consistent snapshot,
	// rather than again for every profile and routing class.
	publishedRows, e := tx.QueryContext(ctx, `SELECT o.config_profile_id::text,o.panel_id::text,cr.route_class,count(DISTINCT o.client_id) FROM output_config_snapshots o JOIN panel_instances p ON p.id=o.panel_id
 JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id
 CROSS JOIN residential_routing_control rc LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id
 LEFT JOIN panel_client_routes cr ON cr.panel_id=p.id AND cr.client_id=o.client_id `+where+` AND o.config_profile_id IS NOT NULL GROUP BY o.config_profile_id,o.panel_id,cr.route_class`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	publishedPanels := map[string]map[string]int{}
	publishedClasses := map[string]map[string]int{}
	for publishedRows.Next() {
		var id, panel, class string
		var n int
		if e = publishedRows.Scan(&id, &panel, &class, &n); e != nil {
			break
		}
		if publishedPanels[id] == nil {
			publishedPanels[id] = map[string]int{}
			publishedClasses[id] = map[string]int{}
		}
		publishedPanels[id][panel] += n
		publishedClasses[id][class] += n
	}
	if e == nil {
		e = publishedRows.Err()
	}
	publishedRows.Close()
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	for _, id := range ids {
		p, err := configplane.Read(ctx, tx, id)
		if err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		panelCounts := []map[string]any{}
		for panel, n := range publishedPanels[id] {
			panelCounts = append(panelCounts, map[string]any{"panel_id": panel, "n": n})
		}
		publishedJSON, _ := json.Marshal(panelCounts)
		var eligible, verified, pending, retrying, ready, links, deleting int
		e = tx.QueryRowContext(ctx, `WITH published AS (SELECT * FROM jsonb_to_recordset($4::jsonb) AS x(panel_id uuid,n bigint)), created AS (SELECT c.profile_id,c.panel_id,count(*) n FROM config_driver_clients c JOIN config_driver_listeners l ON l.profile_id=c.profile_id AND l.panel_id=c.panel_id JOIN config_driver_profiles p ON p.id=c.profile_id WHERE c.confirmed AND l.applied_revision=p.revision AND l.mutation_state='CONFIRMED' AND (c.expires_at IS NULL OR c.expires_at>now()) GROUP BY c.profile_id,c.panel_id) SELECT count(*),
   count(*) FILTER(WHERE l.state='VERIFIED' AND l.applied_revision=cp.revision AND l.verified_at>now()-interval '5 minutes'),
   count(*) FILTER(WHERE COALESCE(l.state,'PENDING')<>'BLOCKED' AND (COALESCE(pub.n,0)<$2 OR COALESCE(made.n,0)<$3 OR COALESCE(l.applied_revision,0)<>cp.revision OR l.state IS DISTINCT FROM 'VERIFIED' OR l.verified_at IS NULL OR l.verified_at<=now()-interval '5 minutes')),
   count(*) FILTER(WHERE l.state='BLOCKED' OR COALESCE(l.error_code,'')<>''),
   count(*) FILTER(WHERE COALESCE(pub.n,0)=$2 AND COALESCE(made.n,0)=$3 AND l.state='VERIFIED' AND l.applied_revision=cp.revision AND l.verified_at>now()-interval '5 minutes'),COALESCE(sum(pub.n),0)
   FROM config_driver_profiles cp CROSS JOIN panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id
   JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id
   LEFT JOIN config_driver_listeners l ON l.profile_id=cp.id AND l.panel_id=pi.id
   LEFT JOIN published pub ON pub.panel_id=pi.id
   LEFT JOIN created made ON made.profile_id=cp.id AND made.panel_id=pi.id
   WHERE cp.id=$1 AND `+configplane.Eligible, id, p.PublishedTarget(), p.Total(), publishedJSON).Scan(&eligible, &verified, &pending, &retrying, &ready, &links)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM config_driver_listeners WHERE profile_id=$1 AND state<>'RETIRED'
  AND ($2 OR NOT $3 OR NOT($4 OR panel_id=ANY($5::uuid[])))`, id, p.DeleteRequestedAt != nil, p.Enabled, p.Fleet, pq.Array(p.Panels)).Scan(&deleting); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		state := "syncing"
		if ready == eligible && eligible > 0 {
			state = "ready"
		}
		if eligible == 0 {
			state = "waiting_for_servers"
		}
		if retrying > 0 {
			state = "retrying"
		}
		if !p.Enabled {
			state = "paused"
		}
		if p.DeleteRequestedAt != nil {
			state = "deleting"
		}
		classStats := map[string]any{}
		for _, class := range p.Classes {
			var publishedCount, active int
			publishedCount = publishedClasses[id][class]
			if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM config_driver_clients c JOIN config_driver_listeners l ON l.profile_id=c.profile_id AND l.panel_id=c.panel_id JOIN config_driver_profiles cp ON cp.id=c.profile_id JOIN panel_instances pi ON pi.id=c.panel_id JOIN accounts a ON a.id=pi.account_id JOIN droplets dr ON dr.id=pi.droplet_id JOIN deployments dep ON dep.droplet_id=dr.id WHERE c.profile_id=$1 AND c.route_class=$2 AND c.confirmed AND l.applied_revision=cp.revision AND l.mutation_state='CONFIRMED' AND (c.expires_at IS NULL OR c.expires_at>now()) AND `+configplane.Eligible, id, class).Scan(&active); e != nil {
				writeJSON(w, 500, errorBody())
				return
			}
			policy := p.ClassSettings[class]
			publicationTarget := 0
			if policy.Publish {
				publicationTarget = eligible * policy.Count
			}
			classStats[class] = map[string]any{"active": active, "target": eligible * policy.Count, "published": publishedCount, "publication_target": publicationTarget, "publish": policy.Publish}
		}
		if state == "syncing" && eligible > 0 {
			complete := true
			for _, raw := range classStats {
				v := raw.(map[string]any)
				if v["active"] != v["target"] {
					complete = false
				}
			}
			if complete {
				state = "verifying"
			}
		}
		profiles = append(profiles, map[string]any{"profile": p, "state": state, "eligible": eligible, "verified": verified, "pending": pending, "blocked": retrying, "ready": ready, "published": links, "target": eligible * p.PublishedTarget(), "deleting": deleting, "class_stats": classStats})
	}
	rows, e = tx.QueryContext(ctx, `SELECT pi.id::text,COALESCE(dep.host,''),COALESCE(dr.profile->>'Region',''),dr.state FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN deployments dep ON dep.droplet_id=dr.id WHERE dr.state IN('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' ORDER BY dep.host`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	servers := []map[string]any{}
	for rows.Next() {
		var id, host, region, state string
		if e = rows.Scan(&id, &host, &region, &state); e != nil {
			break
		}
		servers = append(servers, map[string]any{"id": id, "host": host, "region": region, "state": state})
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var resetState any
	if reset.Valid {
		var state string
		var total, done, failed int
		e = tx.QueryRowContext(ctx, `SELECT j.state,count(t.*),count(*) FILTER(WHERE t.state='SUCCEEDED'),count(*) FILTER(WHERE t.state='FAILED') FROM panel_cleanup_jobs j LEFT JOIN panel_cleanup_targets t ON t.job_id=j.id WHERE j.id=$1 GROUP BY j.state`, reset.String).Scan(&state, &total, &done, &failed)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		resetState = map[string]any{"state": state, "total": total, "done": done, "retrying": failed}
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"drivers": configdriver.Builtins().Catalog(), "profiles": profiles, "servers": servers, "issues": []any{}, "automatic": true, "unified": unified, "reset": resetState, "observed_at": observed})
}

func (s *Server) deleteConfigProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Revision int64 `json:"revision"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(new(any)) != io.EOF || body.Revision < 1 {
		writeJSON(w, 400, errorBody())
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	for _, lock := range []int64{628341902732, 180061015} {
		if _, e = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock($1)", lock); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	p, e := configplane.Read(r.Context(), tx, r.PathValue("id"))
	if e != nil {
		writeJSON(w, 404, errorBody())
		return
	}
	if p.DeleteRequestedAt == nil {
		if p.Revision != body.Revision {
			writeJSON(w, 409, map[string]string{"error": "profile_revision_changed"})
			return
		}
		if _, e = tx.ExecContext(r.Context(), "UPDATE config_driver_profiles SET enabled=false,delete_requested_at=now(),revision=revision+1,updated_at=now() WHERE id=$1", p.ID); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, e = tx.ExecContext(r.Context(), "UPDATE config_driver_listeners SET next_attempt_at=now() WHERE profile_id=$1", p.ID); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, e = tx.ExecContext(r.Context(), "INSERT INTO config_driver_events(profile_id,revision,stage) VALUES($1,$2,'delete_requested')", p.ID, p.Revision+1); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 409, map[string]string{"error": "profile_save_unconfirmed"})
		return
	}
	writeJSON(w, 202, map[string]any{"id": p.ID, "state": "deleting"})
}

func (s *Server) configProfileServers(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT pi.id::text,COALESCE(dep.host,''),COALESCE(l.state,'PENDING'),COALESCE(l.stage,'queued'),COALESCE(l.error_code,''),l.verified_at,l.next_attempt_at,COALESCE(l.applied_revision,0),cp.revision,l.attempted_at,l.stage_started_at,COALESCE(l.failures,0),COALESCE(l.applied_spec->>'sni','')
 FROM config_driver_profiles cp CROSS JOIN panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN deployments dep ON dep.droplet_id=dr.id
 LEFT JOIN config_driver_listeners l ON l.profile_id=cp.id AND l.panel_id=pi.id
 WHERE cp.id=$1 AND cp.deleted_at IS NULL AND dr.state IN('READY','EXPIRING') AND (cp.fleet OR pi.id=ANY(cp.panel_ids) OR l.profile_id IS NOT NULL) ORDER BY dep.host`, r.PathValue("id"))
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, host, state, stage, code string
		var at, next, attempted, started sql.NullTime
		var failures int
		var sni string
		var applied, desired int64
		if e = rows.Scan(&id, &host, &state, &stage, &code, &at, &next, &applied, &desired, &attempted, &started, &failures, &sni); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		out = append(out, map[string]any{"panel_id": id, "host": host, "state": state, "stage": stage, "code": code, "verified_at": at.Time, "next_attempt_at": next.Time, "applied_revision": applied, "desired_revision": desired, "attempted_at": attempted.Time, "stage_started_at": started.Time, "failures": failures, "sni": sni})
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"servers": out, "observed_at": time.Now().UTC()})
}

// Explicit deployment operation: switch authority and retire previous configs.
// Repeated calls return the same durable job, never a second destructive plan.
func (s *Server) startUnifiedConfigs(w http.ResponseWriter, r *http.Request) {
	id, e := (cleanup.Store{DB: s.DB}).StartUnified(r.Context())
	if e != nil {
		writeJSON(w, 409, map[string]string{"error": "config_transition_unconfirmed"})
		return
	}
	writeJSON(w, 202, map[string]any{"job_id": id, "state": "automatic_retirement"})
}

func (s *Server) requireLegacyConfig(next http.HandlerFunc) http.HandlerFunc {
	return s.require(func(w http.ResponseWriter, r *http.Request) {
		var unified bool
		if e := s.DB.QueryRowContext(r.Context(), "SELECT unified FROM config_control_authority WHERE singleton").Scan(&unified); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if unified {
			writeJSON(w, http.StatusGone, map[string]string{"error": "legacy_configuration_retired", "detail": "Manage configurations through Configs profiles."})
			return
		}
		next(w, r)
	}, true)
}
