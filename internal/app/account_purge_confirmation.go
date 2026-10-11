package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"
)

// AccountPurgeConfirmation binds consent to one deletion request and its exact
// local/provider identities and native recovery evidence, including terminal
// records that may still carry unknown cloud outcomes. Discovery or state
// transitions invalidate consent even if the displayed live count is unchanged.
type AccountPurgeConfirmation struct {
	RequestedAt   time.Time
	ResourceScope string
}

const accountPurgeScopeSQL = `(SELECT COALESCE(jsonb_agg(scope_key ORDER BY scope_key::text),'[]'::jsonb)::text FROM (
 SELECT jsonb_build_array('droplet',id::text,COALESCE(provider_resource_id,''),state) scope_key FROM droplets WHERE account_id=$1
 UNION ALL
 SELECT jsonb_build_array('resource',id::text,COALESCE(provider_resource_id,''),state,managed) FROM resources WHERE account_id=$1
 UNION ALL
 SELECT jsonb_build_array('operation',id::text,kind,state,COALESCE(resource_id,''),COALESCE(provider_action_id,'')) FROM operations WHERE account_id=$1
 UNION ALL
 SELECT jsonb_build_array('deployment',id::text,COALESCE(provider_id,''),state,COALESCE(profile_snapshot->>'ssh_provider_key_id','')) FROM deployments WHERE account_id=$1
 UNION ALL
 SELECT jsonb_build_array('ssh_key',provider_key_id,state) FROM account_deletion_keys WHERE account_id=$1
) resource_scope)`

func purgeScopeFingerprint(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func (c AccountPurgeConfirmation) matches(requested sql.NullTime, raw string) bool {
	return requested.Valid && !c.RequestedAt.IsZero() && c.RequestedAt.Equal(requested.Time) && len(c.ResourceScope) == 64 && c.ResourceScope == purgeScopeFingerprint(raw)
}
func ReadAccountPurgeConfirmation(ctx context.Context, db *sql.DB, id string) (AccountPurgeConfirmation, error) {
	var c AccountPurgeConfirmation
	if !canonicalPurgeAccountID(id) {
		return c, ErrAccountPurgeConflict
	}
	var requested sql.NullTime
	var raw string
	err := db.QueryRowContext(ctx, "SELECT deletion_requested_at,"+accountPurgeScopeSQL+" FROM accounts WHERE id=$1", id).Scan(&requested, &raw)
	if err != nil {
		return c, err
	}
	if !requested.Valid {
		return c, ErrAccountPurgeConflict
	}
	c.RequestedAt = requested.Time
	c.ResourceScope = purgeScopeFingerprint(raw)
	return c, nil
}
