package residentialperf

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"sort"
	"time"
)

// This manifest is fixed and versioned. It fetches headers of public static
// resources, never advertisements, impressions, tracking pixels or response bodies.
const AdmissionManifest = "vps-chain-head-v1"
const AdmissionGuardURL = "http://www.gstatic.com/generate_204"

var AdmissionTargets = []struct {
	Name, URL string
	Status    int
}{
	{"probe", "https://www.gstatic.com/generate_204", 204},
	{"browserleaks", "https://browserleaks.com/ip", 200},
	{"ads", "https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js", 200},
	{"gpt", "https://securepubads.g.doubleclick.net/tag/js/gpt.js", 200},
}

type AdmissionContext struct {
	PanelID       string           `json:"panel_id"`
	PanelIdentity string           `json:"panel_identity"`
	Revision      int64            `json:"revision"`
	Plan          string           `json:"plan"`
	Generation    int64            `json:"generation"`
	Owner         string           `json:"owner"`
	Proxies       map[string]int64 `json:"proxy_versions"`
}
type AdmissionObservation struct {
	ProxyID      string    `json:"proxy_id"`
	Target       string    `json:"target"`
	Round        int       `json:"round"`
	Started      time.Time `json:"started"`
	Finished     time.Time `json:"finished"`
	Outcome      string    `json:"outcome"`
	CurlCode     int       `json:"curl_code"`
	HTTPStatus   int       `json:"http_status"`
	Milliseconds int64     `json:"milliseconds"`
}
type AdmissionGuard struct {
	Positive          *AdmissionObservation `json:"positive,omitempty"`
	Denied            *AdmissionObservation `json:"denied,omitempty"`
	NegativeCoreAlive bool                  `json:"negative_core_alive"`
	Startup           string                `json:"startup"`
}

func (g AdmissionGuard) Verified() bool {
	return g.Startup == "ready" && g.Positive != nil && g.Positive.Outcome == "ok" && g.Positive.HTTPStatus == 204 && g.Positive.CurlCode == 0 && g.Denied != nil && g.Denied.HTTPStatus == 0 && (g.Denied.CurlCode == 56 || g.Denied.CurlCode == 52) && g.Denied.Outcome == "transport" && g.NegativeCoreAlive
}

type AdmissionEvidence struct {
	CollectionHealthy bool           `json:"collection_healthy"`
	CollectionOutcome string         `json:"collection_outcome"`
	Guard             AdmissionGuard `json:"guard"`

	ID            string                 `json:"id"`
	Manifest      string                 `json:"manifest"`
	Context       AdmissionContext       `json:"context"`
	Suspects      []string               `json:"suspects"`
	Controls      []string               `json:"controls"`
	Started       time.Time              `json:"started"`
	Finished      time.Time              `json:"finished"`
	ContextStable bool                   `json:"context_stable"`
	ChainVerified bool                   `json:"chain_verified"`
	Observations  []AdmissionObservation `json:"observations"`
}
type admissionQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func admissionContext(ctx context.Context, db admissionQuery, panel string) (AdmissionContext, error) {
	c := AdmissionContext{PanelID: panel}
	var versions []byte
	err := db.QueryRowContext(ctx, `SELECT md5(jsonb_build_array(pi.account_id,pi.droplet_id,pi.driver,pi.base_url,pi.auth_secret_ref)::text),rc.revision,r.plan_hash,p.generation,p.experiment_id::text,
 COALESCE((SELECT jsonb_object_agg(proxy_id::text,admission_version) FROM residential_proxies WHERE enabled),'{}'::jsonb)
 FROM panel_instances pi JOIN panel_routing_state r ON r.panel_id=pi.id JOIN residential_performance_panels p ON p.panel_id=pi.id
 CROSS JOIN residential_routing_control rc WHERE pi.id=$1 AND pi.enabled AND rc.enabled AND (rc.fleet OR pi.id=ANY(rc.panel_ids))`, panel).Scan(&c.PanelIdentity, &c.Revision, &c.Plan, &c.Generation, &c.Owner, &versions)
	if err == nil {
		err = json.Unmarshal(versions, &c.Proxies)
	}
	return c, err
}
func (s Store) AdmissionContext(ctx context.Context, panel string) (AdmissionContext, error) {
	return admissionContext(ctx, s.DB, panel)
}
func (a AdmissionContext) Same(b AdmissionContext) bool { return string(raw(a)) == string(raw(b)) }
func admissionIDs(e AdmissionEvidence) ([]string, error) {
	if !UUID.MatchString(e.ID) || e.Manifest != AdmissionManifest || !UUID.MatchString(e.Context.PanelID) || len(e.Suspects) < 1 || len(e.Suspects) > 2 || len(e.Controls) != 2 {
		return nil, bad("invalid admission manifest or identities")
	}
	ids := append(append([]string{}, e.Suspects...), e.Controls...)
	seen := map[string]bool{}
	for _, id := range ids {
		if !UUID.MatchString(id) || seen[id] || e.Context.Proxies[id] < 1 {
			return nil, bad("probe roles require distinct enabled proxy identities")
		}
		seen[id] = true
	}
	sort.Strings(ids)
	return ids, nil
}

// Eligible distinguishes a complete diagnostic receipt from actionable evidence.
// The oldest scheduled observation sets freshness; later retries cannot reset it.
// Trusted diagnostics remain stored, but inconsistent transport fields cannot authorize.
func consistentObservation(o AdmissionObservation, expected int) bool {
	switch o.Outcome {
	case "ok":
		return o.CurlCode == 0 && o.HTTPStatus == expected
	case "http":
		return o.CurlCode == 0 && o.HTTPStatus >= 100 && o.HTTPStatus <= 599 && o.HTTPStatus != expected
	case "timeout":
		return o.CurlCode == 28 && o.HTTPStatus >= 0 && o.HTTPStatus <= 599
	case "tls":
		return (o.CurlCode == 35 || o.CurlCode == 60) && o.HTTPStatus == 0
	case "local_proxy_unavailable":
		return o.CurlCode == 7 && o.HTTPStatus == 0
	case "auth":
		return o.CurlCode == 67 && o.HTTPStatus == 0
	case "transport":
		return o.CurlCode > 0 && o.CurlCode != 7 && o.CurlCode != 28 && o.CurlCode != 35 && o.CurlCode != 60 && o.CurlCode != 67 && o.HTTPStatus >= 0 && o.HTTPStatus <= 599
	default:
		return false
	}
}

func (e AdmissionEvidence) Eligible(now time.Time) error {
	ids, err := admissionIDs(e)
	if err != nil {
		return err
	}
	if !e.ContextStable || !e.CollectionHealthy || e.CollectionOutcome != "completed" || !e.ChainVerified || !e.Guard.Verified() || e.Started.IsZero() || e.Finished.Before(e.Started) || e.Finished.After(now.Add(time.Second)) || now.Sub(e.Started) > 5*time.Minute {
		return conflict("admission evidence is stale, unstable or lacks chain proof")
	}
	if len(e.Observations) != len(ids)*len(AdmissionTargets)*3 {
		return conflict("admission evidence has incomplete attempt coverage")
	}
	roles := map[string]bool{}
	for _, id := range ids {
		roles[id] = true
	}
	slots := map[string]bool{}
	fails := map[string]int{}
	controls := map[string]bool{}
	for _, id := range e.Controls {
		controls[id] = true
	}
	for _, o := range e.Observations {
		expected := 0
		for _, t := range AdmissionTargets {
			if o.Target == t.Name {
				expected = t.Status
			}
		}
		key := fmt.Sprintf("%s/%s/%d", o.ProxyID, o.Target, o.Round)
		if !roles[o.ProxyID] || expected == 0 || o.Round < 0 || o.Round > 2 || slots[key] || o.Started.Before(e.Started) || o.Finished.Before(o.Started) || o.Finished.After(e.Finished) || o.Milliseconds < 0 {
			return conflict("invalid admission attempt identity or timing")
		}
		slots[key] = true
		if !consistentObservation(o, expected) {
			return conflict("inconsistent admission transport observation")
		}
		ok := o.Outcome == "ok" && o.HTTPStatus == expected && o.CurlCode == 0
		if controls[o.ProxyID] && !ok {
			return conflict("both controls must pass every scheduled destination check")
		}
		if !ok && (o.Outcome == "timeout" || o.Outcome == "tls" || o.Outcome == "http") {
			fails[o.ProxyID+"/"+o.Target]++
		}
	}
	for _, id := range e.Suspects {
		if fails[id+"/ads"] != 3 && fails[id+"/gpt"] != 3 {
			return conflict("suspect lacks repeated failure of the same Ads destination")
		}
	}
	return nil
}

// RecordAdmission is a trusted operator collector sink. There is deliberately no
// HTTP endpoint accepting client-supplied observations. Receipts cannot be edited.
func (s Store) RecordAdmission(ctx context.Context, e AdmissionEvidence) error {
	ids, err := admissionIDs(e)
	if err != nil {
		return err
	}
	if len(e.Observations) != len(ids)*12 || e.Started.IsZero() || e.Finished.Before(e.Started) || e.Finished.Sub(e.Started) > 7*time.Minute {
		return bad("invalid bounded probe receipt")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = Lock(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO residential_admission_evidence(id,panel_id,evidence) VALUES($1,$2,$3)", e.ID, e.Context.PanelID, string(raw(e)))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func loadAdmission(ctx context.Context, db admissionQuery, id string) (AdmissionEvidence, error) {
	var e AdmissionEvidence
	var b []byte
	err := db.QueryRowContext(ctx, "SELECT evidence FROM residential_admission_evidence WHERE id=$1", id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return e, conflict("trusted admission receipt not found")
	}
	if err == nil {
		err = json.Unmarshal(b, &e)
	}
	return e, err
}
func (s Store) AdmissionEvidence(ctx context.Context, id string) (AdmissionEvidence, error) {
	return loadAdmission(ctx, s.DB, id)
}
func admissionStart(ctx context.Context, tx *sql.Tx, q Request, authorization *admissionAuthorization) error {
	// Also serialize proxy insertions/enabling and native-plan writers, not just existing enabled rows.
	// All admission locks are bounded by Store.Do's lock/statement timeouts.
	if _, err := tx.ExecContext(ctx, "LOCK TABLE residential_proxies IN SHARE MODE"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "SELECT 1 FROM panel_routing_state WHERE panel_id=$1 FOR SHARE", q.PanelIDs[0]); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "SELECT 1 FROM panel_instances WHERE id=$1 FOR SHARE", q.PanelIDs[0]); err != nil {
		return err
	}
	// Hold mutable lifecycle eligibility through assignment commitment. Any inverse
	// writer lock order fails closed through the existing three-second lock timeout.
	if _, err := tx.ExecContext(ctx, "SELECT a.id FROM accounts a JOIN panel_instances p ON p.account_id=a.id WHERE p.id=$1 FOR SHARE OF a", q.PanelIDs[0]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "SELECT d.id FROM droplets d JOIN panel_instances p ON p.droplet_id=d.id WHERE p.id=$1 FOR SHARE OF d", q.PanelIDs[0]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "SELECT 1 FROM residential_routing_control WHERE singleton FOR SHARE"); err != nil {
		return err
	}
	e, err := loadAdmission(ctx, tx, q.AdmissionEvidenceID)
	if err != nil {
		return err
	}
	var latest string
	if err = tx.QueryRowContext(ctx, "SELECT id::text FROM residential_admission_evidence WHERE panel_id=$1 ORDER BY recorded_at DESC,id DESC LIMIT 1", e.Context.PanelID).Scan(&latest); err != nil {
		return err
	}
	if latest != q.AdmissionEvidenceID {
		return conflict("a newer diagnostic receipt superseded this evidence")
	}
	now, err := clockNow(ctx, tx)
	if err != nil {
		return err
	}
	if err = e.Eligible(now); err != nil {
		return err
	}
	if err = stabilityStart(ctx, tx, q, e, now); err != nil {
		return err
	}
	if e.Context.PanelID != q.PanelIDs[0] || e.Context.Owner != q.ExperimentID || e.Context.Revision != q.BaseRevision || e.Context.Plan != q.BasePlan {
		return conflict("admission receipt belongs to another panel or plan")
	}
	excluded := append([]string{}, e.Suspects...)
	sort.Strings(excluded)
	if string(raw(excluded)) != string(raw(q.Config.ExcludedProxyIDs)) {
		return bad("exclusions must match the receipt suspect set")
	}
	current, err := admissionContext(ctx, tx, q.PanelIDs[0])
	if err != nil {
		return err
	}
	if !current.Same(e.Context) {
		return conflict("admission context changed since probing")
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM residential_proxies WHERE proxy_id=ANY($1::uuid[]) AND enabled AND type='socks5' AND status='healthy' AND last_success_at>clock_timestamp()-interval '3 minutes'", pq.Array(e.Controls)).Scan(&count); err != nil {
		return err
	}
	if count != 2 {
		return conflict("two verified controls must remain currently eligible")
	}
	var used bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM residential_performance_operations WHERE admission_evidence_id=$1)", q.AdmissionEvidenceID).Scan(&used); err != nil {
		return err
	}
	if used {
		return conflict("admission receipt already used")
	}
	// Assignment mutation preserves these timestamps, but retain their pre-mutation
	// values explicitly so a later proof cannot retroactively authorize the start.
	if err = tx.QueryRowContext(ctx, "SELECT p.verified_at,r.verified_at,d.expires_at FROM residential_performance_panels p JOIN panel_routing_state r ON r.panel_id=p.panel_id JOIN panel_instances pi ON pi.id=p.panel_id JOIN droplets d ON d.id=pi.droplet_id WHERE p.panel_id=$1", q.PanelIDs[0]).Scan(&authorization.performance, &authorization.native, &authorization.expires); err != nil {
		return err
	}
	authorization.evidence = e
	authorization.captured = true
	return nil
}
func admissionStillValid(ctx context.Context, tx *sql.Tx, t *Tuning) (bool, error) {
	e, err := loadAdmission(ctx, tx, t.AdmissionEvidenceID)
	if err != nil {
		var x *Error
		if errors.As(err, &x) {
			return false, nil
		}
		return false, err
	}
	if len(t.Panels) != 1 || t.Panels[0] != e.Context.PanelID {
		return false, nil
	}
	c, err := admissionContext(ctx, tx, t.Panels[0])
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Only the controller-bound candidate hash may replace the reviewed parent hash.
	expectedPlan := e.Context.Plan
	if t.AdmissionPlan != "" {
		expectedPlan = t.AdmissionPlan
	}
	if c.Plan != expectedPlan {
		return false, nil
	}
	if t.AdmissionPlan == "" {
		var applied int64
		if err = tx.QueryRowContext(ctx, "SELECT applied_generation FROM residential_performance_panels WHERE panel_id=$1", t.Panels[0]).Scan(&applied); err != nil {
			return false, err
		}
		if applied != e.Context.Generation {
			return false, nil
		}
	}
	c.Plan = e.Context.Plan
	c.Generation--
	return c.Same(e.Context), nil
}

// ValidateAdmissionAssignment is called before building a candidate and again at
// the panel API mutation boundary. A pending restoration cannot reapply a trial.
func (s Store) ValidateAdmissionAssignment(ctx context.Context, panel string, generation int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	var current int64
	if err = tx.QueryRowContext(ctx, "SELECT experiment_id::text,generation FROM residential_performance_panels WHERE panel_id=$1", panel).Scan(&owner, &current); err != nil {
		return err
	}
	if current != generation {
		return conflict("admission assignment generation changed")
	}
	t, err := loadTuning(ctx, tx, owner)
	if err != nil {
		return err
	}
	if t == nil || t.Phase != "TESTING" || t.AdmissionEvidenceID == "" {
		return conflict("admission trial is no longer testing")
	}
	now, err := clockNow(ctx, tx)
	if err != nil {
		return err
	}
	if !now.Before(t.Deadline) {
		return conflict("admission deadline expired")
	}
	valid, err := admissionStillValid(ctx, tx, t)
	if err != nil {
		return err
	}
	if !valid {
		return conflict("admission source context changed")
	}
	if len(t.Panels) != 1 || t.Panels[0] != panel {
		return conflict("admission panel changed")
	}
	return nil
}

// BindAdmissionPlanTx is called only by the routing controller, under its panel
// config lock and the existing performance advisory transaction lock, before
// persisting the candidate plan. A different candidate cannot replace this hash.
func (s Store) BindAdmissionPlanTx(ctx context.Context, tx *sql.Tx, panel string, generation int64, hash string) error {
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT experiment_id::text FROM residential_performance_panels WHERE panel_id=$1 AND generation=$2", panel, generation).Scan(&owner); err != nil {
		return err
	}
	t, err := loadTuning(ctx, tx, owner)
	if err != nil {
		return err
	}
	if t == nil || t.Phase != "TESTING" || t.AdmissionEvidenceID == "" || len(t.Panels) != 1 || t.Panels[0] != panel {
		return conflict("admission plan owner changed")
	}
	now, err := clockNow(ctx, tx)
	if err != nil {
		return err
	}
	if !now.Before(t.Deadline) {
		return conflict("admission plan deadline expired")
	}
	valid, err := admissionStillValid(ctx, tx, t)
	if err != nil {
		return err
	}
	if !valid {
		return conflict("admission plan source changed")
	}
	if t.AdmissionPlan != "" {
		if t.AdmissionPlan != hash {
			return conflict("admission candidate plan changed")
		}
		return nil
	}
	if hash == "" {
		return bad("empty admission plan")
	}
	t.AdmissionPlan = hash
	return saveTuning(ctx, tx, owner, t)
}

// Retained authorization data, scoped to a single Store.Do transaction.
// Supported proof writers use the same performance lock; lifecycle, routing
// and proxy locks remain held until Do commits or rolls back.
type admissionAuthorization struct {
	captured                     bool
	performance, native, expires sql.NullTime
	evidence                     AdmissionEvidence
}

func (a admissionAuthorization) validate(ctx context.Context, tx *sql.Tx, q Request) error {
	if !a.captured {
		return conflict("admission authorization was not captured")
	}
	now, err := clockNow(ctx, tx)
	if err != nil {
		return err
	}
	for _, proof := range []sql.NullTime{a.performance, a.native} {
		if !proof.Valid || proof.Time.After(now) || !proof.Time.After(now.Add(-time.Minute)) {
			return conflict("admission runtime proof expired before commitment")
		}
	}
	if a.expires.Valid && !a.expires.Time.After(now.Add(time.Duration(q.Minutes+5)*time.Minute)) {
		return conflict("admission server lifetime expired before commitment")
	}
	if err = a.evidence.Eligible(now); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM residential_proxies WHERE proxy_id=ANY($1::uuid[]) AND enabled AND type='socks5' AND status='healthy' AND last_success_at>$2", pq.Array(a.evidence.Controls), now.Add(-3*time.Minute)).Scan(&count); err != nil {
		return err
	}
	if count != 2 {
		return conflict("admission controls expired before commitment")
	}
	return nil
}
