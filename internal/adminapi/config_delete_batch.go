package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/lib/pq"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type profileDeleteItem struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

var profileDeleteID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// The request freezes the exact reviewed set. A retry cannot include a newly
// created profile; stale edits conflict independently. Existing per-listener
// journals perform bounded parallel removal and keep ports reserved until absence.
func (s *Server) deleteConfigProfiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Profiles []profileDeleteItem `json:"profiles"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(new(any)) != io.EOF || len(body.Profiles) < 1 || len(body.Profiles) > 1000 {
		writeJSON(w, 400, errorBody())
		return
	}
	ids := []string{}
	seen := map[string]bool{}
	for i, p := range body.Profiles {
		p.ID = strings.ToLower(p.ID)
		body.Profiles[i] = p
		if !profileDeleteID.MatchString(p.ID) || p.Revision < 1 || seen[p.ID] {
			writeJSON(w, 400, errorBody())
			return
		}
		seen[p.ID] = true
		ids = append(ids, p.ID)
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
	rows, e := tx.QueryContext(r.Context(), "SELECT id::text,revision,delete_requested_at FROM config_driver_profiles WHERE id=ANY($1::uuid[]) FOR UPDATE", pq.Array(ids))
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	type current struct {
		revision int64
		deleting bool
	}
	known := map[string]current{}
	for rows.Next() {
		var id string
		var rev int64
		var at sql.NullTime
		if e = rows.Scan(&id, &rev, &at); e != nil {
			break
		}
		known[id] = current{rev, at.Valid}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	changes := []string{}
	results := []map[string]any{}
	conflicts := 0
	for _, p := range body.Profiles {
		c, ok := known[p.ID]
		state := "deleting"
		if !ok {
			state = "not_found"
			conflicts++
		} else if !c.deleting {
			if c.revision != p.Revision {
				state = "revision_conflict"
				conflicts++
			} else {
				changes = append(changes, p.ID)
			}
		}
		results = append(results, map[string]any{"id": p.ID, "state": state})
	}
	if len(changes) > 0 {
		_, e = tx.ExecContext(r.Context(), `WITH changed AS (
   UPDATE config_driver_profiles SET enabled=false,delete_requested_at=now(),revision=revision+1,updated_at=now()
   WHERE id=ANY($1::uuid[]) RETURNING id,revision
  ) INSERT INTO config_driver_events(profile_id,revision,stage) SELECT id,revision,'delete_requested' FROM changed`, pq.Array(changes))
		if e == nil {
			_, e = tx.ExecContext(r.Context(), "UPDATE config_driver_listeners SET next_attempt_at=now() WHERE profile_id=ANY($1::uuid[])", pq.Array(changes))
		}
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	// Commit the authenticated actor and the exact reviewed snapshot together
	// with its intent. A response retry is a separate audit attempt, never a
	// second native deletion or an expansion to newly-created profiles.
	actor, _ := principal(r.Context())
	result := "ok"
	if conflicts > 0 {
		result = "partial"
	}
	metadata, _ := json.Marshal(map[string]any{
		"profiles": body.Profiles, "results": results,
		"accepted": len(results) - conflicts, "conflicts": conflicts,
		"new_intents": len(changes),
	})
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO audit_events(actor,action,resource_type,result,metadata)
 VALUES($1,'config_profiles_delete_requested','config_profile_batch',$2,$3::jsonb)`, actor.Username, result, string(metadata)); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 409, map[string]string{"error": "profile_save_unconfirmed"})
		return
	}
	writeJSON(w, 202, map[string]any{"results": results, "conflicts": conflicts, "accepted": len(results) - conflicts})
}
