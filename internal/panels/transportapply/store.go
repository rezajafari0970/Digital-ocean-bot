package transportapply

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"strings"
)

var ErrBusy = errors.New("transport application is already pending for a listener")

type QueueResult struct {
	Queued    int `json:"queued"`
	Unchanged int `json:"unchanged"`
}

func Queue(ctx context.Context, tx *sql.Tx, policy transportprofile.Policy, panels ...string) (QueueResult, error) {
	result := QueueResult{}
	if e := policy.Config.Validate(false); e != nil {
		return result, e
	}
	// Serialize admissions and keep the exact requested policy revision immutable.
	if _, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(180061015)"); e != nil {
		return result, e
	}
	// Historic TCP listeners predate persistent pins. Adopt only eligible,
	// managed TCP inventory; the worker still validates fresh native state.
	if _, e := tx.ExecContext(ctx, `INSERT INTO reality_listener_profiles(panel_id,managed_key,config,policy_revision)
 SELECT i.panel_id,i.remark,'{"network":"tcp","finalmask_enabled":false}'::jsonb,$1
 FROM panel_inbound_inventory i JOIN panel_instances pi ON pi.id=i.panel_id
 JOIN droplets d ON d.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=d.id
 WHERE i.remark LIKE 'dob:global-reality:%' AND i.transport='tcp' AND i.protocol='vless' AND i.security='reality' AND i.present AND i.enabled
 AND pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN('READY','EXPIRING') AND dep.state='PANEL_COMPLETE'
 AND (d.expires_at IS NULL OR d.expires_at>now()+interval '2 minutes')
 AND (COALESCE(array_length($2::text[],1),0)=0 OR pi.id::text=ANY($2::text[]))
 ON CONFLICT(panel_id,managed_key) DO NOTHING`, policy.Revision, pq.Array(panels)); e != nil {
		return result, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT p.panel_id::text,p.managed_key,p.config
 FROM reality_listener_profiles p JOIN panel_instances pi ON pi.id=p.panel_id
 JOIN droplets d ON d.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id
 JOIN deployments dep ON dep.droplet_id=d.id
 WHERE p.managed_key LIKE 'dob:global-reality:%' AND pi.enabled AND a.enabled
 AND a.provider_state='ACTIVE' AND d.state IN('READY','EXPIRING') AND dep.state='PANEL_COMPLETE'
 AND (d.expires_at IS NULL OR d.expires_at>now()+interval '2 minutes')
 AND EXISTS(SELECT 1 FROM panel_inbound_inventory i WHERE i.panel_id=p.panel_id AND i.remark=p.managed_key AND i.present AND i.enabled AND i.protocol='vless' AND i.security='reality')
 ORDER BY p.panel_id,p.managed_key FOR UPDATE OF p`)
	if e != nil {
		return result, e
	}
	type item struct {
		panel, key string
		raw        []byte
	}
	items := []item{}
	for rows.Next() {
		var x item
		if e = rows.Scan(&x.panel, &x.key, &x.raw); e != nil {
			rows.Close()
			return result, e
		}
		items = append(items, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	allowed := map[string]bool{}
	for _, p := range panels {
		allowed[p] = true
	}
	for _, x := range items {
		if len(allowed) > 0 && !allowed[x.panel] {
			continue
		}
		var before transportprofile.Config
		if json.Unmarshal(x.raw, &before) != nil {
			return result, transportprofile.ErrInvalid
		}
		target, e := Target(before, policy.Config)
		if e != nil {
			return result, e
		}
		if jsonEqual(before, target) {
			result.Unchanged++
			continue
		}
		var active bool
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM reality_transport_apply_jobs WHERE panel_id=$1 AND managed_key=$2 AND state IN('PENDING','RUNNING','BLOCKED'))", x.panel, x.key).Scan(&active); e != nil {
			return result, e
		}
		if active {
			return result, ErrBusy
		}
		if e = enqueue(ctx, tx, x.panel, x.key, policy.Revision, before, target, nil); e != nil {
			return result, e
		}
		result.Queued++
	}
	return result, nil
}
func Target(before, defaults transportprofile.Config) (transportprofile.Config, error) {
	target := defaults
	if target.NetworkName() == "xhttp" {
		if before.NetworkName() == "xhttp" && len(before.Path) == target.PathBytes {
			target.Path = before.Path
		} else {
			var e error
			target.Path, e = transportprofile.NewPath(target.PathBytes)
			if e != nil {
				return target, e
			}
		}
	}
	return target, target.Validate(target.NetworkName() == "xhttp")
}
func enqueue(ctx context.Context, tx *sql.Tx, panel, key string, revision int64, before, target transportprofile.Config, rollback any, nativeSource ...transportprofile.Config) error {
	if !strings.HasPrefix(key, "dob:global-reality:") {
		return errors.New("unmanaged listener")
	}
	id, e := sanaei.UUIDv4()
	if e != nil {
		return e
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(target)
	source := before
	if len(nativeSource) > 0 {
		source = nativeSource[0]
	}
	c, _ := json.Marshal(source)
	_, e = tx.ExecContext(ctx, `INSERT INTO reality_transport_apply_jobs(id,panel_id,managed_key,policy_revision,before_config,target_config,rollback_of,source_config,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $9 THEN 'RUNNING' ELSE 'PENDING' END)`, id, panel, key, revision, a, b, rollback, c, len(nativeSource) > 0)
	return e
}
func Rollback(ctx context.Context, tx *sql.Tx, id string) error {
	if _, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(180061015)"); e != nil {
		return e
	}
	var panel, key, state, ref string
	var revision int64
	var beforeRaw, targetRaw, currentRaw, sourceRaw []byte
	e := tx.QueryRowContext(ctx, `SELECT j.panel_id::text,j.managed_key,j.policy_revision,j.before_config,j.target_config,p.config,j.state,j.before_secret_ref,j.source_config
 FROM reality_transport_apply_jobs j JOIN reality_listener_profiles p ON p.panel_id=j.panel_id AND p.managed_key=j.managed_key
 WHERE j.id=$1 AND j.state IN('SUCCEEDED','BLOCKED') FOR UPDATE OF p,j`, id).Scan(&panel, &key, &revision, &beforeRaw, &targetRaw, &currentRaw, &state, &ref, &sourceRaw)
	if e != nil {
		return e
	}
	var before, target, current, source transportprofile.Config
	if json.Unmarshal(beforeRaw, &before) != nil || json.Unmarshal(targetRaw, &target) != nil || json.Unmarshal(currentRaw, &current) != nil || json.Unmarshal(sourceRaw, &source) != nil {
		return ErrConflict
	}
	if state == "BLOCKED" {
		if !jsonEqual(current, before) {
			return ErrConflict
		}
		// A pre-write refusal has no native effect and can release admission.
		phase := "ROLLBACK_QUEUED"
		if ref == "" && jsonEqual(source, before) {
			phase = "CANCELLED_BEFORE_WRITE"
		}
		if _, e = tx.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET state='FAILED',phase=$2,last_error='',completed_at=now(),updated_at=now() WHERE id=$1", id, phase); e != nil {
			return e
		}
		if ref == "" && jsonEqual(source, before) {
			return nil
		}
		// The original pin is still before, but native state may be target or
		// before. A new journal owns reconciliation, preserving fresh clients.
		if !jsonEqual(source, before) {
			target = source
		}
		return enqueue(ctx, tx, panel, key, revision, current, before, id, target)
	}
	if !jsonEqual(current, target) {
		return ErrConflict
	}
	var active bool
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM reality_transport_apply_jobs WHERE panel_id=$1 AND managed_key=$2 AND state IN('PENDING','RUNNING','BLOCKED'))", panel, key).Scan(&active); e != nil {
		return e
	}
	if active {
		return ErrBusy
	}
	return enqueue(ctx, tx, panel, key, revision, current, before, id)
}

type Status struct {
	ID         string `json:"id"`
	PanelID    string `json:"panel_id"`
	ManagedKey string `json:"managed_key"`
	State      string `json:"state"`
	Phase      string `json:"phase"`
	Error      string `json:"error"`
	Attempts   int    `json:"attempts"`
}

func List(ctx context.Context, db *sql.DB) ([]Status, error) {
	rows, e := db.QueryContext(ctx, `SELECT id::text,panel_id::text,managed_key,state,phase,last_error,attempts FROM reality_transport_apply_jobs ORDER BY created_at DESC,id LIMIT 500`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Status{}
	for rows.Next() {
		var s Status
		if e = rows.Scan(&s.ID, &s.PanelID, &s.ManagedKey, &s.State, &s.Phase, &s.Error, &s.Attempts); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func blocked(ctx context.Context, db *sql.DB, id, code string) error {
	_, e := db.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET state='BLOCKED',phase='REVIEW_REQUIRED',last_error=$2,updated_at=now() WHERE id=$1", id, code)
	if e != nil {
		return e
	}
	return fmt.Errorf("transport apply blocked: %s", code)
}
