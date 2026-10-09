package residentialperf

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

func sameRoles(a, b []string) bool {
	a = append([]string{}, a...)
	b = append([]string{}, b...)
	sort.Strings(a)
	sort.Strings(b)
	return string(raw(a)) == string(raw(b))
}
func failedDestination(e AdmissionEvidence, id, target string) bool {
	n := 0
	for _, o := range e.Observations {
		if o.ProxyID == id && o.Target == target && (o.Outcome == "timeout" || o.Outcome == "tls" || o.Outcome == "http") {
			n++
		}
	}
	return n == 3
}

// ValidateStabilityPair is an additional gate for the bounded supervisor.
// Existing explicit manual admission remains unchanged when no first ID is supplied.
func ValidateStabilityPair(first, second AdmissionEvidence, now time.Time) error {
	if first.ID == second.ID || first.ID == "" || now.Sub(first.Started) > 10*time.Minute || second.Started.Before(first.Finished.Add(time.Minute)) {
		return conflict("stability requires distinct, fresh and temporally separated windows")
	}
	if err := first.Eligible(first.Finished); err != nil {
		return err
	}
	if err := second.Eligible(now); err != nil {
		return err
	}
	if !first.Context.Same(second.Context) || !sameRoles(first.Suspects, second.Suspects) || !sameRoles(first.Controls, second.Controls) {
		return conflict("stability roles or source context changed")
	}
	for _, id := range second.Suspects {
		if !(failedDestination(first, id, "ads") && failedDestination(second, id, "ads")) && !(failedDestination(first, id, "gpt") && failedDestination(second, id, "gpt")) {
			return conflict("stability requires the same Ads destination to fail in both windows")
		}
	}
	return nil
}

// Called under the existing performance transaction lock. A later receipt cannot
// race this decision; the current second receipt is already required to be latest.
func stabilityStart(ctx context.Context, tx *sql.Tx, q Request, second AdmissionEvidence, now time.Time) error {
	if q.StabilityEvidenceID == "" {
		return nil
	}
	var previous string
	if err := tx.QueryRowContext(ctx, "SELECT id::text FROM residential_admission_evidence WHERE panel_id=$1 ORDER BY recorded_at DESC,id DESC OFFSET 1 LIMIT 1", second.Context.PanelID).Scan(&previous); err != nil {
		if err == sql.ErrNoRows {
			return conflict("stability first window is missing")
		}
		return err
	}
	if previous != q.StabilityEvidenceID {
		return conflict("stability requires consecutive immutable windows")
	}
	var tied, distinct int
	if err := tx.QueryRowContext(ctx, "SELECT count(*),count(DISTINCT recorded_at) FROM residential_admission_evidence WHERE panel_id=$1 AND recorded_at IN (SELECT recorded_at FROM residential_admission_evidence WHERE id IN ($2,$3))", second.Context.PanelID, q.StabilityEvidenceID, second.ID).Scan(&tied, &distinct); err != nil {
		return err
	}
	if tied != 2 || distinct != 2 {
		return conflict("ambiguous stability receipt recording order")
	}
	first, err := loadAdmission(ctx, tx, q.StabilityEvidenceID)
	if err != nil {
		return err
	}
	if err = ValidateStabilityPair(first, second, now); err != nil {
		return err
	}
	var used bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM residential_performance_operations WHERE admission_evidence_id=$1)", first.ID).Scan(&used); err != nil {
		return err
	}
	if used {
		return conflict("stability first window already authorized an admission")
	}
	return nil
}
