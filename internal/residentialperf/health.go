package residentialperf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// HealthView is observational only. It cannot authorize admission or mutate
// provider identity, routing, retry policy, endpoint enablement or experiments.
type HealthView struct {
	PanelID                      string         `json:"panel_id"`
	ObservedAt                   time.Time      `json:"observed_at"`
	EvidenceID                   string         `json:"evidence_id,omitempty"`
	EvidenceStarted              time.Time      `json:"evidence_started,omitempty"`
	State                        string         `json:"state"`
	Rows                         []TargetHealth `json:"targets"`
	SourcePath                   string         `json:"source_path"`
	MutationAllowed              bool           `json:"mutation_allowed"`
	NewProbeRequests             int            `json:"new_probe_requests"`
	ProviderIndependenceVerified bool           `json:"provider_independence_verified"`
}
type TargetHealth struct {
	ProxyID         string `json:"proxy_id"`
	Target          string `json:"target"`
	Role            string `json:"role"`
	State           string `json:"state"`
	Attempts        int    `json:"attempts"`
	Passed          int    `json:"passed"`
	Failed          int    `json:"failed"`
	TimingAvailable int    `json:"timing_available"`
}

func ObserveHealth(current AdmissionContext, evidence *AdmissionEvidence, now time.Time) HealthView {
	v := HealthView{PanelID: current.PanelID, ObservedAt: now, State: "NO_EVIDENCE", Rows: []TargetHealth{}, SourcePath: "diagnostic chain through VPS; not native pool selection or application success"}
	if evidence == nil {
		return v
	}
	e := *evidence
	v.EvidenceID = e.ID
	v.EvidenceStarted = e.Started
	v.State = "INVALID_EVIDENCE"
	ids, err := admissionIDs(e)
	if err != nil || !e.ContextStable || !e.CollectionHealthy || e.CollectionOutcome != "completed" || !e.ChainVerified || !e.Guard.Verified() || e.Started.IsZero() || e.Finished.Before(e.Started) || e.Finished.After(now.Add(time.Second)) || len(e.Observations) != len(ids)*len(AdmissionTargets)*3 {
		return v
	}
	roles := map[string]string{}
	for _, id := range e.Suspects {
		roles[id] = "suspect"
	}
	for _, id := range e.Controls {
		roles[id] = "control"
	}
	slots := map[string]bool{}
	rows := map[string]*TargetHealth{}
	for _, id := range ids {
		for _, target := range AdmissionTargets {
			key := id + "/" + target.Name
			rows[key] = &TargetHealth{ProxyID: id, Target: target.Name, Role: roles[id], State: "OBSERVED_PASS"}
		}
	}
	for _, o := range e.Observations {
		expected := 0
		for _, target := range AdmissionTargets {
			if target.Name == o.Target {
				expected = target.Status
			}
		}
		key := o.ProxyID + "/" + o.Target
		slot := fmt.Sprintf("%s/%d", key, o.Round)
		row := rows[key]
		if row == nil || expected == 0 || o.Round < 0 || o.Round > 2 || slots[slot] || o.Started.Before(e.Started) || o.Finished.Before(o.Started) || o.Finished.After(e.Finished) || o.Milliseconds < 0 || !consistentObservation(o, expected) {
			return v
		}
		slots[slot] = true
		row.Attempts++
		if o.Outcome == "ok" {
			row.Passed++
		} else {
			row.Failed++
			row.State = "OBSERVED_FAILURE"
		}
		if o.Timing != nil {
			row.TimingAvailable++
		}
	}
	v.State = "CURRENT_DIAGNOSTIC"
	if !current.Same(e.Context) {
		v.State = "CONTEXT_CHANGED"
	} else if now.Sub(e.Started) > 5*time.Minute {
		v.State = "STALE"
	}
	for _, id := range ids {
		for _, target := range AdmissionTargets {
			row := *rows[id+"/"+target.Name]
			if v.State != "CURRENT_DIAGNOSTIC" {
				row.State = v.State
			}
			v.Rows = append(v.Rows, row)
		}
	}
	return v
}

// HealthSnapshot reads one current context and its latest immutable receipt in
// one bounded repeatable-read transaction. It performs no probing or writes.
func (s Store) HealthSnapshot(ctx context.Context, panel string) (HealthView, error) {
	if !UUID.MatchString(panel) {
		return HealthView{}, bad("panel UUID required")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return HealthView{}, err
	}
	defer tx.Rollback()
	current, err := admissionContext(ctx, tx, panel)
	if err != nil {
		return HealthView{}, err
	}
	var raw []byte
	var evidence *AdmissionEvidence
	err = tx.QueryRowContext(ctx, "SELECT evidence FROM residential_admission_evidence WHERE panel_id=$1 ORDER BY recorded_at DESC,id DESC LIMIT 1", panel).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return HealthView{}, err
	}
	if err == nil {
		var e AdmissionEvidence
		if err = json.Unmarshal(raw, &e); err != nil {
			return HealthView{}, err
		}
		evidence = &e
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return HealthView{}, err
	}
	view := ObserveHealth(current, evidence, now)
	if err = tx.Commit(); err != nil {
		return HealthView{}, err
	}
	return view, nil
}
