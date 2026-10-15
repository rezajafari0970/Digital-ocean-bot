package ipam

import (
	"context"
	"database/sql"
)

// Account advisory fence precedes row locks; no remote IO is performed under these
// locks. Existing SQLRepository.Put and secret_ref/profile UPDATE/DELETE writers
// conflict with FOR SHARE, including upsert updates. Absent profiles block execution.
func lockGeneration(ctx context.Context, tx *sql.Tx, account string) error {
	var id string
	if e := tx.QueryRowContext(ctx, "SELECT a.id::text FROM accounts a JOIN secrets s ON s.account_id=a.id AND s.id=a.secret_ref WHERE a.id=$1 FOR SHARE OF a,s", account).Scan(&id); e != nil {
		return ErrEvidence
	}
	if e := tx.QueryRowContext(ctx, "SELECT account_id::text FROM network_profiles WHERE account_id=$1 FOR SHARE", account).Scan(&id); e != nil {
		return ErrEvidence
	}
	return nil
}
func currentObservationGeneration(ctx context.Context, tx *sql.Tx, account, generation string) error {
	if generation == "" {
		return ErrEvidence
	}
	if e := lockGeneration(ctx, tx, account); e != nil {
		return e
	}
	current, e := CredentialGeneration(ctx, tx, account)
	if e != nil || current != generation {
		return ErrEvidence
	}
	return nil
}
