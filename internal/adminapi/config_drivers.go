package adminapi

import (
	"encoding/json"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"io"
	"net/http"
	"reflect"
)

func (s *Server) getConfigProfiles(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), "SELECT id::text FROM config_driver_profiles ORDER BY name,id")
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			writeJSON(w, 500, errorBody())
			return
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	profiles := []map[string]any{}
	for _, id := range ids {
		p, e := configplane.Read(r.Context(), s.DB, id)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		var eligible, verified, pending, blocked int
		e = s.DB.QueryRowContext(r.Context(), `SELECT count(*),count(*) FILTER(WHERE l.state='VERIFIED' AND l.applied_revision=cp.revision AND l.verified_at>now()-interval '5 minutes' AND (cp.spec->>'security'<>'tls' OR l.certificate_not_after>now()+interval '5 minutes')),
 count(*) FILTER(WHERE l.state IS DISTINCT FROM 'BLOCKED' AND NOT COALESCE(l.state='VERIFIED' AND l.applied_revision=cp.revision AND l.verified_at>now()-interval '5 minutes' AND (cp.spec->>'security'<>'tls' OR l.certificate_not_after>now()+interval '5 minutes'),false)),
 count(*) FILTER(WHERE l.state='BLOCKED')
 FROM config_driver_profiles cp CROSS JOIN panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id
 LEFT JOIN config_driver_listeners l ON l.profile_id=cp.id AND l.panel_id=pi.id WHERE cp.id=$1 AND `+configplane.Eligible, id).Scan(&eligible, &verified, &pending, &blocked)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		profiles = append(profiles, map[string]any{"profile": p, "eligible": eligible, "verified": verified, "pending": pending, "blocked": blocked})
	}
	rows, e = s.DB.QueryContext(r.Context(), `SELECT l.profile_id::text,l.panel_id::text,l.state,l.stage,l.error_code,l.verified_at FROM config_driver_listeners l
 JOIN config_driver_profiles cp ON cp.id=l.profile_id JOIN panel_instances pi ON pi.id=l.panel_id
 JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id
 WHERE l.state='BLOCKED' AND `+configplane.Eligible+` ORDER BY l.attempted_at DESC LIMIT 100`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	issues := []map[string]any{}
	for rows.Next() {
		var id, panel, state, stage, code string
		var at any
		if rows.Scan(&id, &panel, &state, &stage, &code, &at) != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		issues = append(issues, map[string]any{"profile_id": id, "panel_id": panel, "state": state, "stage": stage, "code": code, "verified_at": at})
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"drivers": configdriver.Builtins().Catalog(), "profiles": profiles, "issues": issues, "automatic": true})
}
func (s *Server) saveConfigProfile(w http.ResponseWriter, r *http.Request) {
	var p configplane.Profile
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	dec.DisallowUnknownFields()
	if dec.Decode(&p) != nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_profile_request"})
		return
	}
	automaticPort := p.Spec.Port == 0
	if automaticPort {
		p.Spec.Port = 10000
	}
	if e := configplane.Validate(p); e != nil {
		writeJSON(w, 400, map[string]string{"error": e.Error()})
		return
	}
	creating := r.Method == "POST"
	if creating {
		if p.ID != "" || p.Revision != 0 || p.Legacy != "" {
			writeJSON(w, 400, errorBody())
			return
		}
		var e error
		p.ID, e = sanaei.UUIDv4()
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	} else if p.ID != r.PathValue("id") || p.Revision < 1 {
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
	if automaticPort {
		e = tx.QueryRowContext(r.Context(), `SELECT n FROM generate_series(10000,60000) n WHERE n<>18080
  AND NOT EXISTS(SELECT 1 FROM config_driver_profiles WHERE (spec->>'port')::integer=n)
  AND NOT EXISTS(SELECT 1 FROM config_driver_listeners WHERE state<>'RETIRED' AND (applied_spec->>'port')::integer=n)
  AND NOT EXISTS(SELECT 1 FROM panel_inbound_inventory WHERE present AND port=n)
  AND NOT EXISTS(SELECT 1 FROM xui_panel_deployments WHERE port=n)
  AND NOT EXISTS(SELECT 1 FROM trojan_tcp_policy WHERE enabled AND port=n)
  AND NOT EXISTS(SELECT 1 FROM (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') p CROSS JOIN LATERAL jsonb_array_elements_text(p.ports) port WHERE port::integer=n)
  ORDER BY n LIMIT 1`).Scan(&p.Spec.Port)
		if e != nil {
			writeJSON(w, 409, map[string]string{"error": "profile_no_free_port"})
			return
		}
	}
	var cleanup bool
	if e = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED'))").Scan(&cleanup); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if p.Enabled && cleanup {
		writeJSON(w, 409, map[string]string{"error": "cleanup_in_progress"})
		return
	}
	if !creating {
		current, e := configplane.Read(r.Context(), tx, p.ID)
		if e != nil {
			writeJSON(w, 404, errorBody())
			return
		}
		if p.Revision != current.Revision {
			writeJSON(w, 409, map[string]string{"error": "profile_revision_changed", "detail": "Reload this profile before saving."})
			return
		}
		p.Legacy = current.Legacy
		var exists, busy bool
		e = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM config_driver_clients WHERE profile_id=$1),EXISTS(SELECT 1 FROM config_driver_listeners WHERE profile_id=$1 AND mutation_state IN('PREPARED','SENT'))", p.ID).Scan(&exists, &busy)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if busy && p.Enabled {
			writeJSON(w, 409, map[string]string{"error": "profile_application_pending", "detail": "The previous change is still being verified."})
			return
		}
		if exists && (p.Count != current.Count || !reflect.DeepEqual(p.Classes, current.Classes) || p.Spec.Protocol != current.Spec.Protocol) {
			writeJSON(w, 409, map[string]string{"error": "profile_identity_settings_retained", "detail": "Use New for a different protocol or client membership. Existing credentials are preserved; port and transport can be changed here."})
			return
		}
	}
	if p.Panels == nil {
		p.Panels = []string{}
	}
	// Inventory includes disabled inbounds; their sockets and ownership cannot be stolen.
	var conflict bool
	e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM config_driver_profiles cp WHERE cp.id<>$1 AND cp.enabled AND (cp.spec->>'port')::integer=$2 AND ($3 OR cp.fleet OR cp.panel_ids&&$4::uuid[]))
 OR EXISTS(SELECT 1 FROM config_driver_listeners l WHERE l.profile_id<>$1 AND l.state<>'RETIRED' AND (l.applied_spec->>'port')::integer=$2 AND ($3 OR l.panel_id=ANY($4::uuid[])))
 OR EXISTS(SELECT 1 FROM (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') x CROSS JOIN LATERAL jsonb_array_elements_text(x.ports) port WHERE port::integer=$2)
 OR EXISTS(SELECT 1 FROM panel_inbound_inventory i JOIN panel_instances pi ON pi.id=i.panel_id JOIN droplets dr ON dr.id=pi.droplet_id WHERE i.present AND i.port=$2 AND dr.state IN('READY','EXPIRING') AND ($3 OR pi.id=ANY($4::uuid[])) AND NOT EXISTS(SELECT 1 FROM config_driver_listeners l WHERE l.profile_id=$1 AND l.panel_id=i.panel_id AND l.remote_id=i.remote_id AND l.managed_key=i.remark))
 OR EXISTS(SELECT 1 FROM xui_panel_deployments x JOIN droplets dr ON dr.id=x.droplet_id WHERE x.port=$2 AND dr.state IN('READY','EXPIRING'))`, p.ID, p.Spec.Port, p.Fleet, pq.Array(p.Panels)).Scan(&conflict)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if p.Enabled && conflict {
		writeJSON(w, 409, map[string]string{"error": "profile_port_conflict", "detail": "Choose a free port. Another profile, listener or panel reserves this port."})
		return
	}
	raw, _ := json.Marshal(p.Spec)
	if creating {
		p.Revision = 1
		_, e = tx.ExecContext(r.Context(), "INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,panel_ids,classes,clients_per_class) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", p.ID, p.Name, raw, p.Enabled, p.Fleet, pq.Array(p.Panels), pq.Array(p.Classes), p.Count)
	} else {
		p.Revision++
		_, e = tx.ExecContext(r.Context(), "UPDATE config_driver_profiles SET name=$2,spec=$3,enabled=$4,fleet=$5,panel_ids=$6,classes=$7,clients_per_class=$8,revision=$9,updated_at=now() WHERE id=$1", p.ID, p.Name, raw, p.Enabled, p.Fleet, pq.Array(p.Panels), pq.Array(p.Classes), p.Count, p.Revision)
	}
	if e != nil {
		writeJSON(w, 409, map[string]string{"error": "profile_save_conflict"})
		return
	}
	if _, e = tx.ExecContext(r.Context(), "INSERT INTO config_driver_events(profile_id,revision,stage) VALUES($1,$2,'desired_saved')", p.ID, p.Revision); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE config_driver_listeners SET next_attempt_at=now() WHERE profile_id=$1", p.ID); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 409, map[string]string{"error": "profile_save_unconfirmed", "detail": "Reload profiles to verify the saved revision before retrying."})
		return
	}
	writeJSON(w, 200, map[string]any{"profile": p, "state": "queued"})
}
