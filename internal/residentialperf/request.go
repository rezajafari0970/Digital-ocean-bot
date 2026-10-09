package residentialperf

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func normalizeRequest(q Request) (Request, error) {
	if q.Config != nil {
		c := q.Config.Clone()
		q.Config = &c
	}
	q.StabilityEvidenceID = strings.ToLower(q.StabilityEvidenceID)
	if q.StabilityEvidenceID != "" && (q.Action != "tune_start" || q.AdmissionEvidenceID == "" || !UUID.MatchString(q.StabilityEvidenceID)) {
		return q, bad("stability evidence requires an admission start")
	}
	q.AdmissionEvidenceID = strings.ToLower(q.AdmissionEvidenceID)
	if q.AdmissionEvidenceID != "" && (q.Action != "tune_start" || !UUID.MatchString(q.AdmissionEvidenceID)) {
		return q, bad("admission evidence is only accepted for tune_start")
	}
	if !UUID.MatchString(q.RequestID) {
		return q, bad("request_id must be a UUID")
	}
	q.RequestID = strings.ToLower(q.RequestID)
	q.ExperimentID = strings.ToLower(q.ExperimentID)
	// Normalize an owned copy: concurrent callers may reuse the same request slice.
	q.PanelIDs = append([]string(nil), q.PanelIDs...)
	seen := map[string]bool{}
	for i, id := range q.PanelIDs {
		id = strings.ToLower(id)
		if !UUID.MatchString(id) || seen[id] {
			return q, bad("panel IDs must be unique UUIDs")
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
		if q.Config != nil && len(q.Config.ExcludedProxyIDs) > 0 {
			return q, bad("exclusions require an evidence-bound timed tuning trial")
		}
		if q.Config == nil || q.BaseRevision < 1 || len(q.PanelIDs) > 1000 {
			return q, bad("configuration, reviewed revision and at most 1000 selected servers required")
		}
		if mode != "timed" && mode != "permanent" {
			return q, bad("mode must be timed or permanent")
		}
		if scope != "selected" && scope != "fleet" {
			return q, bad("scope must be selected or fleet")
		}
		if scope == "selected" && len(q.PanelIDs) == 0 {
			return q, bad("select at least one server")
		}
		if scope == "fleet" && (mode != "permanent" || len(q.PanelIDs) > 0) {
			return q, bad("fleet scope requires permanent mode and a server-resolved target list")
		}
		if mode == "timed" && (q.Minutes < 5 || q.Minutes > 60) {
			return q, bad("timed tests require a 5–60 minute deadline")
		}
		if mode == "permanent" && q.Minutes != 0 {
			return q, bad("permanent publication has no timer")
		}
		if e := q.Config.Validate(); e != nil {
			return q, bad(e.Error())
		}
	} else if !UUID.MatchString(q.ExperimentID) {
		return q, bad("experiment_id must be a UUID")
	}

	return q, nil
}
func normalizedRequestHash(q Request) string { return fmt.Sprintf("%x", sha256.Sum256(raw(q))) }

// RequestHash shares Store.Do's exact validation, normalization and JSON representation.
func RequestHash(q Request) (string, error) {
	q, err := normalizeRequest(q)
	if err != nil {
		return "", err
	}
	return normalizedRequestHash(q), nil
}

// Reconcile only reads an immutable operation. A matching ID is not sufficient:
// the complete canonical request and admission identity must match.
func (s Store) Reconcile(ctx context.Context, q Request) (*Receipt, error) {
	q, err := normalizeRequest(q)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var stored string
	var response []byte
	var evidence sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT request_hash,response,admission_evidence_id::text FROM residential_performance_operations WHERE request_id=$1", q.RequestID).Scan(&stored, &response, &evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if stored != normalizedRequestHash(q) || evidence.String != q.AdmissionEvidenceID {
		return nil, conflict("saved request does not match committed operation")
	}
	var receipt Receipt
	if err = json.Unmarshal(response, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}
