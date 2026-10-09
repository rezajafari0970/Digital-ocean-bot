package residentialperf

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"strings"
	"time"
)

// Tuning is a latest-only recovery record. Starting another trial replaces the
// previous publication's recovery point, only after all its assignments verify.
type Tuning struct {
	AdmissionPlan       string    `json:"admission_plan,omitempty"`
	AdmissionEvidenceID string    `json:"admission_evidence_id,omitempty"`
	Schema              int       `json:"schema"`
	ID                  string    `json:"id"`
	Phase               string    `json:"phase"`
	Before              Config    `json:"before"`
	Candidate           Config    `json:"candidate"`
	Panels              []string  `json:"panels"`
	Deadline            time.Time `json:"deadline"`
	BaseVersion         int64     `json:"base_version"`
	BaseRevision        int64     `json:"base_revision"`
	RestoreScope        string    `json:"restore_scope,omitempty"`
	Assigned            bool      `json:"assigned"`
	Rejected            bool      `json:"rejected"`
	Reason              string    `json:"reason,omitempty"`
	Conflict            string    `json:"conflict,omitempty"`
}

func tuningBusy(t *Tuning) bool {
	return t != nil && (t.Phase == "TESTING" || t.Phase == "PUBLISHING" || t.Phase == "RESTORING")
}
func loadTuning(ctx context.Context, tx *sql.Tx, id string) (*Tuning, error) {
	var b []byte
	if e := tx.QueryRowContext(ctx, "SELECT tuning FROM residential_performance_experiments WHERE id=$1", id).Scan(&b); e != nil {
		return nil, e
	}
	if len(b) == 0 || string(b) == "null" {
		return nil, nil
	}
	var t Tuning
	if e := json.Unmarshal(b, &t); e != nil {
		return nil, e
	}
	if t.Schema != 1 {
		return nil, conflict("unsupported tuning recovery record")
	}
	return &t, nil
}
func saveTuning(ctx context.Context, tx *sql.Tx, id string, t *Tuning) error {
	_, e := tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET tuning=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$1", id, nullable(t))
	return e
}
func clockNow(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var now time.Time
	e := tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now)
	return now, e
}
func tuningConfigChange(before, candidate Config) error {
	if e := candidate.Validate(); e != nil {
		return bad(e.Error())
	}
	if len(before.ExcludedProxyIDs) > 0 {
		return bad("parent profile cannot contain exclusions")
	}
	if len(candidate.ExcludedProxyIDs) > 0 {
		a := candidate.Clone()
		a.ExcludedProxyIDs = nil
		if string(raw(a)) != string(raw(before)) {
			return bad("admission can change only excluded proxies")
		}
		return nil
	}
	a := candidate
	a.FastCount = before.FastCount
	a.FastShare = before.FastShare
	if string(raw(a)) != string(raw(before)) {
		return bad("tuning can change only fast_count and fast_share")
	}
	if candidate.FastCount == before.FastCount && candidate.FastShare == before.FastShare {
		return bad("tuning must change the selection")
	}
	return nil
}

// Only genuinely deleted inventory is exempt. Expired, disabled and unreachable
// surviving panels retain their recovery obligations.
const tuningLive = ` EXISTS(SELECT 1 FROM panel_instances pi JOIN droplets d ON d.id=pi.droplet_id WHERE pi.id=t.panel_id AND d.state<>'DELETED') `
const tuningFresh = ` t.state='APPLIED' AND p.experiment_id=t.experiment_id
 AND p.generation=t.generation AND p.applied_generation=t.generation
 AND p.verified_at>clock_timestamp()-interval '60 seconds'
 AND r.state='APPLIED' AND r.revision=c.revision
 AND r.performance_generation=t.generation
 AND r.verified_at>clock_timestamp()-interval '60 seconds' `

// Saved selected IDs and every surviving fleet target are obligations. A missing
// or changed marker is a conflict, never a way to shrink the proof set.
func tuningCoverage(ctx context.Context, tx *sql.Tx, id string, t *Tuning, fleet bool) error {
	var missing bool
	query := `SELECT EXISTS(SELECT 1 FROM unnest($3::uuid[]) selected(panel_id)
 WHERE EXISTS(SELECT 1 FROM panel_instances pi JOIN droplets d ON d.id=pi.droplet_id WHERE pi.id=selected.panel_id AND d.state<>'DELETED')
 AND NOT EXISTS(SELECT 1 FROM residential_performance_targets target WHERE target.experiment_id=$1 AND target.panel_id=selected.panel_id
 AND target.tuning_id=$2 AND target.tuning_generation=target.generation))`
	if e := tx.QueryRowContext(ctx, query, id, t.ID, pq.Array(t.Panels)).Scan(&missing); e != nil {
		return e
	}
	if missing {
		return conflict("selected tuning identity or generation marker changed")
	}
	if fleet {
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM residential_performance_panels p
  JOIN panel_instances pi ON pi.id=p.panel_id JOIN droplets d ON d.id=pi.droplet_id
  WHERE p.experiment_id=$1 AND d.state<>'DELETED' AND NOT EXISTS(SELECT 1 FROM residential_performance_targets t WHERE t.experiment_id=$1 AND t.panel_id=p.panel_id))`, id).Scan(&missing); e != nil {
			return e
		}
		if missing {
			return conflict("owned fleet assignment has lost its target recovery record")
		}
		e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM residential_performance_targets t WHERE t.experiment_id=$1 AND `+tuningLive+`
  AND (t.tuning_id IS DISTINCT FROM $2::uuid OR t.tuning_generation IS DISTINCT FROM t.generation))`, id, t.ID).Scan(&missing)
		if e != nil {
			return e
		}
		if missing {
			return conflict("fleet tuning identity or generation marker changed")
		}
	}
	return nil
}
func tuningVerified(ctx context.Context, tx *sql.Tx, id string, t *Tuning, all bool) (bool, error) {
	if e := tuningCoverage(ctx, tx, id, t, all); e != nil {
		return false, e
	}
	var pending bool
	filter := " AND t.panel_id=ANY($3::uuid[])"
	if all {
		filter = " AND $3::uuid[] IS NOT NULL"
	}
	query := `SELECT EXISTS(SELECT 1 FROM residential_performance_targets t
 LEFT JOIN residential_performance_panels p ON p.panel_id=t.panel_id
 LEFT JOIN panel_routing_state r ON r.panel_id=t.panel_id
 CROSS JOIN residential_routing_control c
 WHERE t.experiment_id=$1` + filter + " AND " + tuningLive + " AND NOT COALESCE((" + tuningFresh +
		` AND t.tuning_id=$2::uuid AND t.tuning_generation=t.generation),false))`
	e := tx.QueryRowContext(ctx, query, id, t.ID, pq.Array(t.Panels)).Scan(&pending)
	return !pending, e
}

// Every operator batch is validated before any assignment changes, including
// owner/config/generation for untargeted surviving assignments.
func tuningOwnership(ctx context.Context, tx *sql.Tx, id string, t *Tuning, source string, selected bool) error {
	var orphan bool
	if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM residential_performance_panels p
 JOIN panel_instances pi ON pi.id=p.panel_id JOIN droplets d ON d.id=pi.droplet_id
 WHERE p.experiment_id=$1 AND d.state<>'DELETED' AND NOT EXISTS(SELECT 1 FROM residential_performance_targets target WHERE target.experiment_id=$1 AND target.panel_id=p.panel_id))`, id).Scan(&orphan); e != nil {
		return e
	}
	if orphan {
		return conflict("owned assignment has lost its target recovery record")
	}
	if selected || source == "candidate" || t.Phase == "RESTORING" || t.Phase == "PUBLISHING" || t.Phase == "PUBLISHED" {
		if e := tuningCoverage(ctx, tx, id, t, !selected); e != nil {
			return e
		}
	}
	var drift bool
	filter := ""
	if selected {
		filter = " AND t.tuning_id=$2::uuid"
	}
	wanted := string(raw(t.Before))
	if source == "candidate" {
		wanted = string(raw(t.Candidate))
	}
	// During publication the selected trial has candidate; all others retain before.
	expression := "$3::jsonb"
	if source == "publish" {
		expression = "CASE WHEN t.tuning_id=$2::uuid THEN $4::jsonb ELSE $3::jsonb END"
	}
	query := `SELECT EXISTS(SELECT 1 FROM residential_performance_targets t
 LEFT JOIN residential_performance_panels p ON p.panel_id=t.panel_id
 WHERE t.experiment_id=$1` + filter + " AND " + tuningLive + `
 AND (p.experiment_id IS DISTINCT FROM t.experiment_id OR p.generation IS DISTINCT FROM t.generation
 OR p.config IS DISTINCT FROM ` + expression + `
 OR (t.tuning_id=$2::uuid AND t.tuning_generation IS DISTINCT FROM t.generation)))`
	// Refer to $4 even when unused so argument typing is deterministic.
	query += " AND $4::jsonb IS NOT NULL"
	e := tx.QueryRowContext(ctx, query, id, t.ID, wanted, string(raw(t.Candidate))).Scan(&drift)
	if e != nil {
		return e
	}
	if drift {
		return conflict("tuning assignment ownership, configuration or generation changed")
	}
	return nil
}

func tuningAssign(ctx context.Context, tx *sql.Tx, id string, t *Tuning, c Config, selected bool) error {
	filter := ""
	if selected {
		filter = " AND t.tuning_id=$2::uuid"
	}
	_, e := tx.ExecContext(ctx, `UPDATE residential_performance_panels p SET
 generation=p.generation+1,config=$3::jsonb,last_error=''
 FROM residential_performance_targets t
 WHERE t.experiment_id=$1 AND p.panel_id=t.panel_id AND p.experiment_id=$1`+filter+
		" AND "+tuningLive+" AND p.config IS DISTINCT FROM $3::jsonb AND $2::uuid IS NOT NULL", id, t.ID, string(raw(c)))
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE residential_performance_targets t SET
 generation=p.generation,tuning_id=$2,tuning_generation=p.generation,
 state=CASE WHEN p.applied_generation=p.generation THEN t.state ELSE 'PENDING' END
 FROM residential_performance_panels p WHERE t.experiment_id=$1 AND p.panel_id=t.panel_id
 AND p.experiment_id=$1`+filter+" AND "+tuningLive, id, t.ID)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM worker_item_failures WHERE kind='residential_sync'
 AND item_id IN(SELECT panel_id::text FROM residential_performance_targets WHERE experiment_id=$1 AND tuning_id=$2)`, id, t.ID)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE panel_routing_state r SET state='PENDING',next_check_at=clock_timestamp()
 FROM residential_performance_targets t JOIN residential_performance_panels p ON p.panel_id=t.panel_id
 WHERE t.experiment_id=$1 AND t.tuning_id=$2 AND r.panel_id=t.panel_id AND p.applied_generation<>p.generation`, id, t.ID)
	return e
}
func tuningRestoreIntent(t *Tuning, scope, reason string) {
	t.Phase = "RESTORING"
	t.RestoreScope = scope
	t.Assigned = false
	t.Reason = reason
	t.Conflict = ""
}

// The recovery intent survives a conflict and unrelated fleet-enrollment errors.
// Only the assignment batch is rolled back; no conflicting owner is overwritten.
func tuningRestoreAssignments(ctx context.Context, tx *sql.Tx, id string, t *Tuning) error {
	if t.Assigned {
		return nil
	}
	if _, e := tx.ExecContext(ctx, "SAVEPOINT tuning_restore"); e != nil {
		return e
	}
	e := tuningOwnership(ctx, tx, id, t, "candidate", t.RestoreScope == "selected")
	if e == nil {
		e = tuningAssign(ctx, tx, id, t, t.Before, t.RestoreScope == "selected")
	}
	if e == nil && t.RestoreScope == "fleet" {
		_, e = tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET spec=$2 WHERE id=$1", id, string(raw(t.Before)))
	}
	if e != nil {
		if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT tuning_restore"); rollbackErr != nil {
			return rollbackErr
		}
		// Avoid exposing database details or erasing the durable recovery obligation.
		t.Conflict = "Recovery pending: assignment conflict or database operation failed"
	} else {
		t.Assigned = true
		t.Conflict = ""
	}
	if _, e = tx.ExecContext(ctx, "RELEASE SAVEPOINT tuning_restore"); e != nil {
		return e
	}
	return saveTuning(ctx, tx, id, t)
}
func tuneAction(ctx context.Context, tx *sql.Tx, q Request, state string, version int64, spec []byte) error {
	if state != "KEPT" {
		return conflict("tuning requires a kept permanent fleet profile")
	}
	var mode, scope string
	if e := tx.QueryRowContext(ctx, "SELECT duration_mode,publish_scope FROM residential_performance_experiments WHERE id=$1", q.ExperimentID).Scan(&mode, &scope); e != nil {
		return e
	}
	if mode != "permanent" || scope != "fleet" {
		return conflict("tuning requires a permanent fleet profile")
	}
	t, e := loadTuning(ctx, tx, q.ExperimentID)
	if e != nil {
		return e
	}
	now, e := clockNow(ctx, tx)
	if e != nil {
		return e
	}
	if q.Action == "tune_start" {
		if tuningBusy(t) {
			return conflict("finish the existing tuning application or restoration first")
		}
		if t != nil && t.Phase == "PUBLISHED" {
			ok, e := tuningVerified(ctx, tx, q.ExperimentID, t, true)
			if e != nil {
				return e
			}
			if !ok {
				return conflict("the previous publication still has unverified assignments")
			}
		}
		if q.TuningID != "" || q.Config == nil || len(q.PanelIDs) < 1 || len(q.PanelIDs) > 3 || q.Minutes < 5 || q.Minutes > 30 || q.BaseRevision < 1 {
			return bad("tuning requires configuration, 1–3 panels, reviewed revision and 5–30 minutes")
		}
		var before Config
		if e = json.Unmarshal(spec, &before); e != nil {
			return e
		}
		if e = tuningConfigChange(before, *q.Config); e != nil {
			return e
		}
		if len(q.Config.ExcludedProxyIDs) > 0 {
			if len(q.PanelIDs) != 1 || q.AdmissionEvidenceID == "" {
				return bad("admission requires exactly one panel and trusted evidence")
			}
			if e = admissionStart(ctx, tx, q); e != nil {
				return e
			}
		} else if q.AdmissionEvidenceID != "" {
			return bad("evidence cannot authorize a selection tuning trial")
		}
		t = &Tuning{AdmissionEvidenceID: q.AdmissionEvidenceID, Schema: 1, ID: q.RequestID, Phase: "TESTING", Before: before, Candidate: *q.Config, Panels: q.PanelIDs, Deadline: now.Add(time.Duration(q.Minutes) * time.Minute), BaseVersion: version, BaseRevision: q.BaseRevision}
		if e = tuningOwnership(ctx, tx, q.ExperimentID, t, "before", false); e != nil {
			return e
		}
		for _, panel := range q.PanelIDs {
			if e = eligibleWallClock(ctx, tx, panel, q.Minutes, q.BaseRevision, q.BasePlan); e != nil {
				return e
			}
			var ok bool
			e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM residential_performance_targets t
   JOIN residential_performance_panels p ON p.panel_id=t.panel_id
   JOIN panel_routing_state r ON r.panel_id=t.panel_id CROSS JOIN residential_routing_control c
   WHERE t.experiment_id=$1 AND t.panel_id=$2 AND `+tuningFresh+")", q.ExperimentID, panel).Scan(&ok)
			if e != nil {
				return e
			}
			if !ok {
				return conflict("selected assignment lacks fresh runtime proof")
			}
		}
		_, e = tx.ExecContext(ctx, "UPDATE residential_performance_targets SET tuning_id=NULL,tuning_generation=0 WHERE experiment_id=$1", q.ExperimentID)
		if e != nil {
			return e
		}
		for _, panel := range q.PanelIDs {
			_, e = tx.ExecContext(ctx, "UPDATE residential_performance_targets SET tuning_id=$3,tuning_generation=generation WHERE experiment_id=$1 AND panel_id=$2", q.ExperimentID, panel, t.ID)
			if e != nil {
				return e
			}
		}
		if e = tuningAssign(ctx, tx, q.ExperimentID, t, t.Candidate, true); e != nil {
			return e
		}
		return saveTuning(ctx, tx, q.ExperimentID, t)
	}
	if t == nil || !UUID.MatchString(q.TuningID) || !strings.EqualFold(q.TuningID, t.ID) {
		return conflict("the intended tuning recovery record changed")
	}
	if q.Config != nil || len(q.PanelIDs) > 0 {
		return bad("lifecycle actions use the saved tuning configuration and target set")
	}
	switch q.Action {
	case "tune_cancel":
		if t.Phase != "TESTING" {
			return conflict("only a testing trial can be cancelled")
		}
		tuningRestoreIntent(t, "selected", "operator cancelled")
		return tuningRestoreAssignments(ctx, tx, q.ExperimentID, t)
	case "tune_publish":
		if len(t.Candidate.ExcludedProxyIDs) > 0 || t.AdmissionEvidenceID != "" {
			return conflict("admission trials cannot be published; allow restoration")
		}
		if t.Phase != "TESTING" || t.Rejected {
			return conflict("only an unrejected testing trial can be published")
		}
		ok, e := tuningVerified(ctx, tx, q.ExperimentID, t, false)
		if e != nil {
			return e
		}
		if !ok {
			return conflict("every trial target needs fresh runtime proof")
		}
		// A disappeared canary is not evidence for publication.
		for _, panel := range t.Panels {
			if e = eligibleWallClock(ctx, tx, panel, 0, t.BaseRevision, ""); e != nil {
				return e
			}
		}
		if e = tuningOwnership(ctx, tx, q.ExperimentID, t, "publish", false); e != nil {
			return e
		}
		now, e = clockNow(ctx, tx)
		if e != nil {
			return e
		}
		if !now.Before(t.Deadline.Add(-time.Minute)) {
			return conflict("publication cutoff reached; cancel or wait for restoration")
		}
		if e = tuningAssign(ctx, tx, q.ExperimentID, t, t.Candidate, false); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "UPDATE residential_performance_experiments SET spec=$2 WHERE id=$1", q.ExperimentID, string(raw(t.Candidate)))
		if e != nil {
			return e
		}
		t.Phase = "PUBLISHING"
		t.Assigned = true
		return saveTuning(ctx, tx, q.ExperimentID, t)
	case "tune_restore":
		if t.Phase != "PUBLISHED" && t.Phase != "PUBLISHING" {
			return conflict("only the latest publication can be restored")
		}
		tuningRestoreIntent(t, "fleet", "operator restored prior publication")
		return tuningRestoreAssignments(ctx, tx, q.ExperimentID, t)
	default:
		return bad("unknown tuning action")
	}
}
func eligibleWallClock(ctx context.Context, tx *sql.Tx, panel string, minutes int, revision int64, plan string) error {
	var ok bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id
 JOIN accounts a ON a.id=p.account_id JOIN panel_routing_state r ON r.panel_id=p.id CROSS JOIN residential_routing_control c
 WHERE p.id=$1 AND p.enabled AND d.state='READY' AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND (d.expires_at IS NULL OR d.expires_at>clock_timestamp()+($2+5)*interval '1 minute')
 AND c.enabled AND (c.fleet OR p.id=ANY(c.panel_ids)) AND r.pool_enabled AND r.state='APPLIED' AND r.revision=c.revision
 AND r.verified_at>clock_timestamp()-interval '60 seconds' AND c.revision=$3 AND ($4='' OR r.plan_hash=$4)
 AND EXISTS(SELECT 1 FROM residential_proxies WHERE enabled AND status='healthy' AND last_success_at>clock_timestamp()-interval '3 minutes'))`, panel, minutes, revision, plan).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return conflict("selected server is not freshly eligible at the reviewed revision")
	}
	return nil
}

// tickTuning commits independently before ordinary enrollment, so a broken new
// enrollment cannot suppress a timed trial's durable recovery.
func (s Store) tickTuning(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='10s'"); e != nil {
		return e
	}
	if e = Lock(ctx, tx); e != nil {
		return e
	}
	var id string
	e = tx.QueryRowContext(ctx, "SELECT id::text FROM residential_performance_experiments WHERE state='KEPT' AND tuning IS NOT NULL FOR UPDATE").Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	t, e := loadTuning(ctx, tx, id)
	if e != nil {
		return e
	}
	if t.Phase == "TESTING" {
		now, e := clockNow(ctx, tx)
		if e != nil {
			return e
		}
		reason := ""
		if !now.Before(t.Deadline) {
			reason = "trial deadline expired"
		}
		if reason == "" && t.AdmissionEvidenceID != "" {
			valid, err := admissionStillValid(ctx, tx, t)
			if err != nil {
				return err
			}
			if !valid {
				reason = "admission context changed"
				t.Rejected = true
			}
		}
		if reason != "" {
			tuningRestoreIntent(t, "selected", reason)
			if e = saveTuning(ctx, tx, id, t); e != nil {
				return e
			}
		}
	}
	if t.Phase == "RESTORING" {
		if e = tuningRestoreAssignments(ctx, tx, id, t); e != nil {
			return e
		}
	}
	if (t.Phase == "RESTORING" && t.Assigned) || t.Phase == "PUBLISHING" {
		source := "candidate"
		selected := false
		if t.Phase == "RESTORING" {
			source = "before"
			selected = t.RestoreScope == "selected"
		}
		e = tuningOwnership(ctx, tx, id, t, source, selected)
		if e != nil {
			var c *Error
			if !errors.As(e, &c) {
				return e
			}
			if t.Conflict == "" {
				t.Conflict = "Recovery or publication pending: assignment drift"
				if e = saveTuning(ctx, tx, id, t); e != nil {
					return e
				}
			}
		} else {
			ok, e := tuningVerified(ctx, tx, id, t, !selected)
			if e != nil {
				return e
			}
			if ok {
				if t.Phase == "RESTORING" {
					t.Phase = "RESTORED"
				} else {
					t.Phase = "PUBLISHED"
				}
				t.Conflict = ""
				if e = saveTuning(ctx, tx, id, t); e != nil {
					return e
				}
			}
		}
	}
	return tx.Commit()
}
func tuningFailure(ctx context.Context, tx *sql.Tx, id, panel string, gen int64) error {
	t, e := loadTuning(ctx, tx, id)
	if e != nil || t == nil {
		return e
	}
	if t.Phase != "TESTING" && t.Phase != "PUBLISHING" {
		return nil
	}
	var match bool
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM residential_performance_targets WHERE experiment_id=$1 AND panel_id=$2 AND tuning_id=$3 AND tuning_generation=$4 AND generation=$4)", id, panel, t.ID, gen).Scan(&match); e != nil {
		return e
	}
	if !match {
		return nil
	}
	scope := "selected"
	if t.Phase == "PUBLISHING" {
		scope = "fleet"
	}
	t.Rejected = true
	tuningRestoreIntent(t, scope, "runtime verification failed")
	return saveTuning(ctx, tx, id, t)
}
