package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

func (s *Server) saveConfigProfile(w http.ResponseWriter, r *http.Request) {
	var p configplane.Profile
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	dec.DisallowUnknownFields()
	if dec.Decode(&p) != nil || dec.Decode(new(any)) != io.EOF || p.DeleteRequestedAt != nil || p.DeletedAt != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_profile_request"})
		return
	}
	if p.RequestID != "" && !regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(p.RequestID) {
		writeJSON(w, 400, errorBody())
		return
	}
	p.Normalize()
	automaticPort := p.Spec.Port == 0
	if automaticPort {
		p.Spec.Port = 10000
		if p.Spec.CloudflareAccount != "" {
			p.Spec.Port = 2083
		}
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
	if creating && p.RequestID != "" {
		var priorID string
		e = tx.QueryRowContext(r.Context(), "SELECT id::text FROM config_driver_profiles WHERE request_id=$1", p.RequestID).Scan(&priorID)
		if e == nil {
			prior, err := configplane.Read(r.Context(), tx, priorID)
			if err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
			requested := p.Spec
			if automaticPort {
				requested.Port = prior.Spec.Port
			}
			if prior.Name != p.Name || prior.Spec != requested || prior.Enabled != p.Enabled || prior.Fleet != p.Fleet || prior.Count != p.Count || !reflect.DeepEqual(prior.ClassSettings, p.ClassSettings) || !reflect.DeepEqual(prior.Classes, p.Classes) || !(len(prior.Panels) == 0 && len(p.Panels) == 0) && !reflect.DeepEqual(prior.Panels, p.Panels) {
				writeJSON(w, 409, map[string]string{"error": "profile_request_already_used"})
				return
			}
			writeJSON(w, 200, map[string]any{"profile": prior, "state": "already_saved"})
			return
		}
		if !errors.Is(e, sql.ErrNoRows) {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if automaticPort {
		portQuery := `SELECT n FROM generate_series(10000,60000) n WHERE n<>18080
  AND NOT EXISTS(SELECT 1 FROM config_driver_profiles WHERE enabled AND delete_requested_at IS NULL AND (spec->>'port')::integer=n)
  AND NOT EXISTS(SELECT 1 FROM config_driver_listeners WHERE state<>'RETIRED' AND ((remote_id IS NOT NULL AND (applied_spec->>'port')::integer=n) OR (mutation_state IN('PREPARED','SENT') AND (mutation_spec->>'port')::integer=n)))
  AND NOT EXISTS(SELECT 1 FROM panel_inbound_inventory WHERE present AND port=n)
  AND NOT EXISTS(SELECT 1 FROM xui_panel_deployments WHERE port=n)
  AND NOT EXISTS(SELECT 1 FROM trojan_tcp_policy WHERE enabled AND port=n)
  AND (EXISTS(SELECT 1 FROM config_control_authority WHERE unified) OR NOT EXISTS(SELECT 1 FROM (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') p CROSS JOIN LATERAL jsonb_array_elements_text(p.ports) port WHERE port::integer=n))
  ORDER BY n LIMIT 1`
		if p.Spec.CloudflareAccount != "" {
			portQuery = strings.Replace(portQuery, "generate_series(10000,60000)", "unnest(ARRAY[443,2053,2083,2087,2096,8443])", 1)
		}
		e = tx.QueryRowContext(r.Context(), portQuery).Scan(&p.Spec.Port)
		if e != nil {
			writeJSON(w, 409, map[string]string{"error": "profile_no_free_port"})
			return
		}
	}
	if p.Spec.CloudflareAccount != "" {
		var ready bool
		if e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloudflare_accounts a JOIN cloudflare_zones z ON z.account_id=a.id WHERE a.id=$1 AND a.enabled AND z.enabled AND z.present AND z.status='active')`, p.Spec.CloudflareAccount).Scan(&ready); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if !ready {
			writeJSON(w, 400, map[string]string{"error": "profile_cloudflare_no_active_zones"})
			return
		}
	}
	var cleanup bool
	if e = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED'))").Scan(&cleanup); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var unified bool
	if e = tx.QueryRowContext(r.Context(), "SELECT unified FROM config_control_authority WHERE singleton").Scan(&unified); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if p.Enabled && cleanup && !unified {
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
		if p.Count != current.Count && reflect.DeepEqual(p.ClassSettings, current.ClassSettings) {
			for _, cl := range p.Classes {
				policy := p.ClassSettings[cl]
				policy.Count = p.Count
				p.ClassSettings[cl] = policy
			}
		}
		p.Legacy = current.Legacy
		p.RequestID = current.RequestID
		if current.DeleteRequestedAt != nil {
			writeJSON(w, 409, map[string]string{"error": "profile_is_deleting"})
			return
		}

	}
	if p.Panels == nil {
		p.Panels = []string{}
	}
	// Inventory includes disabled inbounds; their sockets and ownership cannot be stolen.
	var conflict bool
	e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM config_driver_profiles cp WHERE cp.id<>$1 AND cp.enabled AND cp.delete_requested_at IS NULL AND (cp.spec->>'port')::integer=$2 AND ($3 OR cp.fleet OR cp.panel_ids&&$4::uuid[]))
 OR EXISTS(SELECT 1 FROM config_driver_listeners l WHERE l.profile_id<>$1 AND l.state<>'RETIRED' AND ((l.remote_id IS NOT NULL AND (l.applied_spec->>'port')::integer=$2) OR (l.mutation_state IN('PREPARED','SENT') AND (l.mutation_spec->>'port')::integer=$2)) AND ($3 OR l.panel_id=ANY($4::uuid[])))
 OR (NOT EXISTS(SELECT 1 FROM config_control_authority WHERE unified) AND EXISTS(SELECT 1 FROM (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') x CROSS JOIN LATERAL jsonb_array_elements_text(x.ports) port WHERE port::integer=$2))
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
	policies, _ := json.Marshal(p.ClassSettings)
	if creating {
		p.Revision = 1
		_, e = tx.ExecContext(r.Context(), "INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,panel_ids,classes,clients_per_class,request_id,class_settings) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,$10)", p.ID, p.Name, raw, p.Enabled, p.Fleet, pq.Array(p.Panels), pq.Array(p.Classes), p.Count, p.RequestID, policies)
	} else {
		p.Revision++
		_, e = tx.ExecContext(r.Context(), "UPDATE config_driver_profiles SET name=$2,spec=$3,enabled=$4,fleet=$5,panel_ids=$6,classes=$7,clients_per_class=$8,revision=$9,class_settings=$10,updated_at=now() WHERE id=$1", p.ID, p.Name, raw, p.Enabled, p.Fleet, pq.Array(p.Panels), pq.Array(p.Classes), p.Count, p.Revision, policies)
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
