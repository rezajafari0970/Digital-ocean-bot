package providers

import (
	"context"
	"database/sql"
)

// AbsenceVerifier persists eventual-consistency evidence across driver/process
// restarts. An empty provider read is not immediately authoritative absence.
type AbsenceVerifier interface {
	Confirm(context.Context, string) error
	Present(context.Context, string) error
}
type SQLAbsenceVerifier struct {
	DB                  *sql.DB
	AccountID, Provider string
}

func absencePending() error {
	return &Error{Class: ErrorUnavailable, Operation: "verify_absence", Code: "RESOURCE_ABSENCE_UNCONFIRMED", Message: "provider absence is not yet stable"}
}
func ConfirmAbsence(ctx context.Context, v AbsenceVerifier, id string) error {
	if v == nil {
		return absencePending()
	}
	return v.Confirm(ctx, id)
}
func (v SQLAbsenceVerifier) Present(ctx context.Context, id string) error {
	_, e := v.DB.ExecContext(ctx, "DELETE FROM provider_absence_evidence WHERE account_id=$1 AND provider=$2 AND server_id=$3", v.AccountID, v.Provider, id)
	return e
}
func (v SQLAbsenceVerifier) Confirm(ctx context.Context, id string) error {
	var stable bool
	e := v.DB.QueryRowContext(ctx, `WITH observation AS (
 INSERT INTO provider_absence_evidence(account_id,provider,server_id) VALUES($1,$2,$3)
 ON CONFLICT(account_id,provider,server_id) DO UPDATE SET
 observations=CASE WHEN provider_absence_evidence.last_observed_at<=now()-interval '10 seconds' THEN provider_absence_evidence.observations+1 ELSE provider_absence_evidence.observations END,
 last_observed_at=CASE WHEN provider_absence_evidence.last_observed_at<=now()-interval '10 seconds' THEN now() ELSE provider_absence_evidence.last_observed_at END
 RETURNING first_observed_at,observations
 ) SELECT observations>=3 AND first_observed_at<=now()-interval '2 minutes'
 AND EXISTS(SELECT 1 FROM droplets d JOIN accounts a ON a.id=d.account_id WHERE d.account_id=$1 AND a.provider=$2 AND d.provider_resource_id=$3 AND d.created_at<=now()-interval '5 minutes') FROM observation`, v.AccountID, v.Provider, id).Scan(&stable)
	if e != nil {
		return e
	}
	if !stable {
		return absencePending()
	}
	return nil
}
