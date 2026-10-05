package residentialperf

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

type Store struct{ DB *sql.DB }
type Request struct {
	RequestID       string   `json:"request_id"`
	ExperimentID    string   `json:"experiment_id"`
	Action          string   `json:"action"`
	Config          *Config  `json:"config,omitempty"`
	PanelIDs        []string `json:"panel_ids,omitempty"`
	ExpectedVersion int64    `json:"expected_version"`
	Minutes         int      `json:"minutes"`
	BaseRevision    int64    `json:"base_revision"`
	BasePlan        string   `json:"base_plan"`
	Mode            string   `json:"mode,omitempty"`
	Scope           string   `json:"scope,omitempty"`
}
type Receipt struct {
	ExperimentID string `json:"experiment_id"`
	State        string `json:"state"`
	Version      int64  `json:"version"`
}
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }
func conflict(s string) error  { return &Error{409, s} }
func bad(s string) error       { return &Error{400, s} }
func raw(v any) []byte         { b, _ := json.Marshal(v); return b }
func nullable(v any) any {
	b := raw(v)
	if string(b) == "null" {
		return nil
	}
	return string(b)
}
func Lock(ctx context.Context, tx *sql.Tx) error {
	_, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('residential-performance',0))")
	return e
}
func (s Store) Do(ctx context.Context, q Request) (Receipt, error) {
	var out Receipt
	if !UUID.MatchString(q.RequestID) {
		return out, bad("request_id must be a UUID")
	}
	q.RequestID = strings.ToLower(q.RequestID)
	q.ExperimentID = strings.ToLower(q.ExperimentID)
	seen := map[string]bool{}
	for i, id := range q.PanelIDs {
		id = strings.ToLower(id)
		if !UUID.MatchString(id) || seen[id] {
			return out, bad("panel IDs must be unique UUIDs")
		}
		seen[id] = true
		q.PanelIDs[i] = id
	}
	sort.Strings(q.PanelIDs)
	mode, scope := q.Mode, q.Scope
	if mode == "" {
		mode = "timed"
	}
	if scope == "" {
		scope = "selected"
	}
	if q.Action == "start" {
		if q.Config == nil || q.BaseRevision < 1 || len(q.PanelIDs) > 1000 {
			return out, bad("configuration, reviewed revision and at most 1000 selected servers required")
		}
		if mode != "timed" && mode != "permanent" {
			return out, bad("mode must be timed or permanent")
		}
		if scope != "selected" && scope != "fleet" {
			return out, bad("scope must be selected or fleet")
		}
		if scope == "selected" && len(q.PanelIDs) == 0 {
			return out, bad("select at least one server")
		}
		if scope == "fleet" && (mode != "permanent" || len(q.PanelIDs) > 0) {
			return out, bad("fleet scope requires permanent mode and a server-resolved target list")
		}
		if mode == "timed" && (q.Minutes < 5 || q.Minutes > 60) {
			return out, bad("timed tests require a 5–60 minute deadline")
		}
		if mode == "permanent" && q.Minutes != 0 {
			return out, bad("permanent publication has no timer")
		}
		if e := q.Config.Validate(); e != nil {
			return out, bad(e.Error())
		}
	} else if !UUID.MatchString(q.ExperimentID) {
		return out, bad("experiment_id must be a UUID")
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(raw(q)))
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if e = Lock(ctx, tx); e != nil {
		return out, e
	}
	var oldhash string
	var old []byte
	e = tx.QueryRowContext(ctx, "SELECT request_hash,response FROM residential_performance_operations WHERE request_id=$1", q.RequestID).Scan(&oldhash, &old)
	if e == nil {
		if oldhash != digest {
			return out, conflict("request_id was used for a different operation")
		}
		e = json.Unmarshal(old, &out)
		return out, e
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if q.Action == "start" {
		var busy bool
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM residential_performance_experiments WHERE state IN('RUNNING','KEPT','ROLLING_BACK'))").Scan(&busy); e != nil {
			return out, e
		}
		if busy {
			return out, conflict("roll back the existing profile before starting a different test")
		}
		var revision int64
		if e = tx.QueryRowContext(ctx, "SELECT revision FROM residential_routing_control WHERE singleton FOR SHARE").Scan(&revision); e != nil {
			return out, e
		}
		if revision != q.BaseRevision {
			return out, conflict("routing settings changed; refresh before publishing")
		}
		for _, id := range q.PanelIDs {
			if mode == "timed" {
				plan := ""
				if len(q.PanelIDs) == 1 {
					plan = q.BasePlan
				}
				e = eligible(ctx, tx, id, q.Minutes, q.BaseRevision, plan)
			} else {
				e = publishable(ctx, tx, id)
			}
			if e != nil {
				return out, e
			}
		}
		for id := range q.Config.Costs {
			var ok bool
			if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM residential_proxies WHERE proxy_id=$1 AND enabled)", id).Scan(&ok); e != nil {
				return out, e
			}
			if !ok {
				return out, bad("cost refers to an unavailable proxy")
			}
		}
		q.ExperimentID = q.RequestID
		_, e = tx.ExecContext(ctx, `INSERT INTO residential_performance_experiments(id,spec,state,deadline,duration_mode,publish_scope)
 VALUES($1,$2,CASE WHEN $4='permanent' THEN 'KEPT' ELSE 'RUNNING' END,CASE WHEN $4='permanent' THEN NULL ELSE now()+$3*interval '1 minute' END,$4,$5)`, q.ExperimentID, string(raw(q.Config)), q.Minutes, mode, scope)
		if e != nil {
			return out, e
		}
		if scope == "fleet" {
			e = enrollFleet(ctx, tx, q.ExperimentID, q.Config)
		} else {
			for _, id := range q.PanelIDs {
				if e = attach(ctx, tx, q.ExperimentID, id, q.Config); e != nil {
					break
				}
			}
		}
	} else {
		var state string
		var version int64
		var spec []byte
		var expired bool
		e = tx.QueryRowContext(ctx, "SELECT state,version,spec,COALESCE(deadline<=now(),false) FROM residential_performance_experiments WHERE id=$1 FOR UPDATE", q.ExperimentID).Scan(&state, &version, &spec, &expired)
		if errors.Is(e, sql.ErrNoRows) {
			return out, bad("experiment not found")
		}
		if e != nil {
			return out, e
		}
		if version != q.ExpectedVersion {
			return out, conflict("experiment changed; refresh before this action")
		}
		switch q.Action {
		case "rollback":
			e = rollback(ctx, tx, q.ExperimentID, "requested")
		case "publish":
			if state != "RUNNING" && state != "KEPT" {
				return out, conflict("only an open profile can be published")
			}
			if state == "RUNNING" && expired {
				return out, conflict("the test deadline expired; wait for rollback")
			}
			if q.Scope != "fleet" || q.Config != nil || len(q.PanelIDs) > 0 {
				return out, bad("publish keeps the current profile and targets all current and future servers")
			}
			_, e = tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET state='KEPT',duration_mode='permanent',publish_scope='fleet',deadline=NULL,version=version+1,reason='published permanently',updated_at=now() WHERE id=$1", q.ExperimentID)
			if e == nil {
				var c Config
				if e = json.Unmarshal(spec, &c); e == nil {
					e = enrollFleet(ctx, tx, q.ExperimentID, &c)
				}
			}
		case "keep", "promote":
			if state != "RUNNING" || expired {
				return out, conflict("only an unexpired running experiment can be kept or promoted")
			}
			var unverified bool
			e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM residential_performance_targets t LEFT JOIN residential_performance_panels p ON p.panel_id=t.panel_id LEFT JOIN panel_routing_state r ON r.panel_id=t.panel_id CROSS JOIN residential_routing_control c WHERE t.experiment_id=$1 AND (t.state<>'APPLIED' OR p.experiment_id IS DISTINCT FROM t.experiment_id OR p.applied_generation IS DISTINCT FROM t.generation OR p.verified_at IS NULL OR p.verified_at<now()-interval '60 seconds' OR r.state IS DISTINCT FROM 'APPLIED' OR r.revision IS DISTINCT FROM c.revision OR r.performance_generation IS DISTINCT FROM t.generation))`, q.ExperimentID).Scan(&unverified)
			if e != nil {
				return out, e
			}
			if unverified {
				return out, conflict("every target must have a fresh successful runtime verification")
			}
			if q.Action == "keep" {
				_, e = tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET state='KEPT',version=version+1,updated_at=now() WHERE id=$1", q.ExperimentID)
			} else {
				if len(q.PanelIDs) < 1 || len(q.PanelIDs) > 1000 {
					return out, bad("select 1–1000 eligible servers")
				}
				var c Config
				if e = json.Unmarshal(spec, &c); e != nil {
					return out, e
				}
				for _, id := range q.PanelIDs {
					var exists bool
					e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM residential_performance_targets WHERE experiment_id=$1 AND panel_id=$2)", q.ExperimentID, id).Scan(&exists)
					if e != nil {
						return out, e
					}
					if exists {
						continue
					}
					if e = eligible(ctx, tx, id, 5, 0, ""); e != nil {
						return out, e
					}
					if e = attach(ctx, tx, q.ExperimentID, id, &c); e != nil {
						return out, e
					}
				}
				_, e = tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET version=version+1,updated_at=now() WHERE id=$1", q.ExperimentID)
			}
		default:
			return out, bad("unknown experiment action")
		}
	}
	if e != nil {
		return out, e
	}
	e = tx.QueryRowContext(ctx, "SELECT id::text,state,version FROM residential_performance_experiments WHERE id=$1", q.ExperimentID).Scan(&out.ExperimentID, &out.State, &out.Version)
	if e != nil {
		return out, e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO residential_performance_operations(request_id,request_hash,response) VALUES($1,$2,$3)", q.RequestID, digest, string(raw(out)))
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func eligible(ctx context.Context, tx *sql.Tx, id string, minutes int, revision int64, plan string) error {
	var ok bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN panel_routing_state r ON r.panel_id=p.id CROSS JOIN residential_routing_control c
 WHERE p.id=$1 AND p.enabled AND d.state='READY' AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND (d.expires_at IS NULL OR d.expires_at>now()+($2+5)*interval '1 minute')
 AND c.enabled AND (c.fleet OR p.id=ANY(c.panel_ids)) AND r.pool_enabled AND r.state='APPLIED' AND r.revision=c.revision AND r.verified_at>now()-interval '60 seconds'
 AND ($3::bigint=0 OR c.revision=$3) AND ($4='' OR r.plan_hash=$4)
 AND EXISTS(SELECT 1 FROM residential_proxies WHERE enabled AND status='healthy' AND last_success_at>now()-interval '3 minutes'))`, id, minutes, revision, plan).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return conflict("server is not freshly verified, has changed, or expires too soon for this test")
	}
	return nil
}
func attach(ctx context.Context, tx *sql.Tx, experiment, panel string, c *Config) error {
	var previous []byte
	var gen int64
	e := tx.QueryRowContext(ctx, "SELECT config,generation FROM residential_performance_panels WHERE panel_id=$1 FOR UPDATE", panel).Scan(&previous, &gen)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	gen++
	_, e = tx.ExecContext(ctx, `INSERT INTO residential_performance_panels(panel_id,experiment_id,generation,config) VALUES($1,$2,$3,$4)
 ON CONFLICT(panel_id) DO UPDATE SET experiment_id=excluded.experiment_id,generation=excluded.generation,config=excluded.config,last_error=''`, panel, experiment, gen, string(raw(c)))
	if e != nil {
		return e
	}
	var prior any
	if len(previous) > 0 && string(previous) != "null" {
		prior = string(previous)
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO residential_performance_targets(experiment_id,panel_id,before_config,generation) VALUES($1,$2,$3,$4)", experiment, panel, prior, gen)
	if e == nil {
		_, e = tx.ExecContext(ctx, "DELETE FROM worker_item_failures WHERE kind='residential_sync' AND item_id=$1", panel)
	}
	if e == nil {
		_, e = tx.ExecContext(ctx, "UPDATE panel_routing_state SET next_check_at=now(),state='PENDING' WHERE panel_id=$1", panel)
	}
	return e
}
func rollback(ctx context.Context, tx *sql.Tx, id, reason string) error {
	var state string
	if e := tx.QueryRowContext(ctx, "SELECT state FROM residential_performance_experiments WHERE id=$1 FOR UPDATE", id).Scan(&state); e != nil {
		return e
	}
	if state == "ROLLED_BACK" || state == "ROLLING_BACK" {
		return nil
	}
	var superseded bool
	if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM residential_performance_targets t JOIN residential_performance_panels p ON p.panel_id=t.panel_id WHERE t.experiment_id=$1 AND p.experiment_id<>t.experiment_id)`, id).Scan(&superseded); e != nil {
		return e
	}
	if superseded {
		return conflict("a newer experiment owns these settings; roll back the latest experiment first")
	}
	_, e := tx.ExecContext(ctx, `UPDATE residential_performance_panels p SET config=t.before_config,generation=p.generation+1,last_error='' FROM residential_performance_targets t WHERE t.experiment_id=$1 AND t.panel_id=p.panel_id AND p.experiment_id=$1`, id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE residential_performance_targets t SET state='ROLLBACK_PENDING',generation=p.generation,failures=0 FROM residential_performance_panels p WHERE t.experiment_id=$1 AND t.panel_id=p.panel_id AND p.experiment_id=$1`, id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "DELETE FROM worker_item_failures WHERE kind='residential_sync' AND item_id IN(SELECT panel_id::text FROM residential_performance_targets WHERE experiment_id=$1)", id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE panel_routing_state SET next_check_at=now(),state='PENDING' WHERE panel_id IN(SELECT panel_id FROM residential_performance_targets WHERE experiment_id=$1)", id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET state='ROLLING_BACK',reason=$2,version=version+1,updated_at=now() WHERE id=$1", id, reason)
	return e
}

type Assignment struct {
	Generation                  int64
	Config                      *Config
	Baseline, Observed, Pending *Fields
}

func (s Store) Load(ctx context.Context, panel string) (Assignment, error) {
	var a Assignment
	var c, b, o, p []byte
	e := s.DB.QueryRowContext(ctx, "SELECT generation,config,baseline,observed,pending FROM residential_performance_panels WHERE panel_id=$1", panel).Scan(&a.Generation, &c, &b, &o, &p)
	if errors.Is(e, sql.ErrNoRows) {
		return a, nil
	}
	if e != nil {
		return a, e
	}
	for _, x := range []struct {
		b []byte
		v any
	}{{c, &a.Config}, {b, &a.Baseline}, {o, &a.Observed}, {p, &a.Pending}} {
		if len(x.b) > 0 {
			if e = json.Unmarshal(x.b, x.v); e != nil {
				return a, e
			}
		}
	}
	return a, nil
}
func (s Store) Prepare(ctx context.Context, panel string, generation int64, current Fields) (Assignment, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Assignment{}, e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, "UPDATE residential_performance_panels SET baseline=COALESCE(baseline,$3::jsonb),observed=COALESCE(observed,$3::jsonb) WHERE panel_id=$1 AND generation=$2", panel, generation, string(raw(current)))
	if e != nil {
		return Assignment{}, e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return Assignment{}, conflict("performance generation changed")
	}
	if e = tx.Commit(); e != nil {
		return Assignment{}, e
	}
	a, e := s.Load(ctx, panel)
	if e != nil {
		return a, e
	}
	if a.Generation != generation {
		return a, conflict("performance generation changed")
	}
	return a, CheckOwned(current, a.Baseline, a.Observed, a.Pending)
}
func (s Store) Plan(ctx context.Context, panel string, gen int64, f Fields) error {
	r, e := s.DB.ExecContext(ctx, "UPDATE residential_performance_panels SET pending=$3 WHERE panel_id=$1 AND generation=$2", panel, gen, string(raw(f)))
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return conflict("performance generation changed before mutation")
	}
	return nil
}
func (s Store) Applied(ctx context.Context, panel string, gen int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = Lock(ctx, tx); e != nil {
		return e
	}
	if e = s.AppliedTx(ctx, tx, panel, gen); e != nil {
		return e
	}
	return tx.Commit()
}
func (s Store) AppliedTx(ctx context.Context, tx *sql.Tx, panel string, gen int64) error {
	r, e := tx.ExecContext(ctx, "UPDATE residential_performance_panels SET baseline=CASE WHEN config IS NULL THEN NULL ELSE baseline END,observed=CASE WHEN config IS NULL THEN NULL ELSE pending END,pending=NULL,applied_generation=$2,verified_at=now(),last_error='' WHERE panel_id=$1 AND generation=$2", panel, gen)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return conflict("performance generation changed during verification")
	}
	_, e = tx.ExecContext(ctx, `UPDATE residential_performance_targets t SET state=CASE WHEN t.state IN('ROLLBACK_PENDING','RESTORED') THEN 'RESTORED' ELSE 'APPLIED' END,failures=0 FROM residential_performance_panels p WHERE p.panel_id=$1 AND p.experiment_id=t.experiment_id AND t.panel_id=p.panel_id AND t.generation=$2`, panel, gen)
	if e != nil {
		return e
	}
	return nil
}
func (s Store) Failed(ctx context.Context, panel string, gen int64, cause error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return
	}
	defer tx.Rollback()
	if Lock(ctx, tx) != nil {
		return
	}
	message := "Runtime verification pending"
	if strings.Contains(cause.Error(), "field conflict") {
		message = "Rollback conflict: an owned setting changed outside this experiment"
	}
	var id string
	e = tx.QueryRowContext(ctx, "UPDATE residential_performance_panels SET last_error=$3 WHERE panel_id=$1 AND generation=$2 RETURNING experiment_id::text", panel, gen, message).Scan(&id)
	if e != nil {
		return
	}
	var count int
	e = tx.QueryRowContext(ctx, "UPDATE residential_performance_targets SET failures=failures+1 WHERE experiment_id=$1 AND panel_id=$2 AND generation=$3 RETURNING failures", id, panel, gen).Scan(&count)
	if e != nil {
		return
	}
	if count >= 3 {
		var state string
		if tx.QueryRowContext(ctx, "SELECT state FROM residential_performance_experiments WHERE id=$1", id).Scan(&state) == nil && state == "RUNNING" {
			if rollback(ctx, tx, id, "verification failures") != nil {
				return
			}
		}
	}
	_ = tx.Commit()
}
func (s Store) Tick(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = Lock(ctx, tx); e != nil {
		return e
	}
	var id string
	e = tx.QueryRowContext(ctx, "SELECT id::text FROM residential_performance_experiments WHERE state='RUNNING' AND deadline<=now() LIMIT 1 FOR UPDATE").Scan(&id)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil {
		if e = rollback(ctx, tx, id, "test deadline expired"); e != nil {
			return e
		}
	}
	if e = enrollPublished(ctx, tx); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE residential_performance_targets t SET state='RETIRED' WHERE state IN('PENDING','ROLLBACK_PENDING','APPLIED') AND NOT EXISTS(SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id WHERE p.id=t.panel_id AND d.state<>'DELETED')`)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE residential_performance_experiments e SET state='ROLLED_BACK',version=version+1,updated_at=now() WHERE state='ROLLING_BACK' AND NOT EXISTS(SELECT 1 FROM residential_performance_targets t WHERE t.experiment_id=e.id AND t.state NOT IN('RESTORED','RETIRED'))`)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s Store) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		if e := s.Tick(ctx); e != nil && ctx.Err() == nil {
			log.Print("residential performance deadline reconciliation pending")
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
