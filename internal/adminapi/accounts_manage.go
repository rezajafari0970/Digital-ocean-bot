package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"net/http"
	"strings"
	"time"
)

type accountUpdate struct {
	ApplyToExisting        bool     `json:"apply_to_existing"`
	Name                   string   `json:"name"`
	Token                  string   `json:"token"`
	Regions                []string `json:"regions"`
	Sizes                  []string `json:"sizes"`
	Image                  string   `json:"image"`
	Images                 []string `json:"images"`
	LifetimeSeconds        int      `json:"lifetime_seconds"`
	LifetimeMinSeconds     int      `json:"lifetime_min_seconds"`
	LifetimeMaxSeconds     int      `json:"lifetime_max_seconds"`
	IntervalSeconds        int      `json:"interval_seconds"`
	BuildSpacingMinutes    int      `json:"build_spacing_minutes"`
	BuildSpacingMaxMinutes int      `json:"build_spacing_max_minutes"`
	BatchSize              int      `json:"batch_size"`
	MaxConcurrent          int      `json:"max_concurrent"`
	DesiredServerCount     int      `json:"desired_server_count"`
	FallbackAnyRegion      *bool    `json:"fallback_any_region"`
	NetworkMode            string   `json:"network_mode"`
	ProxyID                string   `json:"proxy_id"`
	ProxyIDs               []string `json:"proxy_ids"`
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x accountUpdate
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	x.ProxyIDs = normalizeProxyPool(x.ProxyID, x.ProxyIDs)
	if x.NetworkMode == "proxy_required" && len(x.ProxyIDs) > 0 {
		x.ProxyID = x.ProxyIDs[0]
	}
	var currentLimit int
	var accountProvider string
	_ = s.DB.QueryRowContext(r.Context(), "SELECT provider FROM accounts WHERE id=$1", id).Scan(&accountProvider)
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE((SELECT COALESCE((ps.canonical->'Capacity'->>'ComputeLimit')::int,(ps.data->'Limits'->>'DropletLimit')::int) FROM provider_snapshots ps WHERE ps.account_id=$1 ORDER BY ps.created_at DESC LIMIT 1),0)`, id).Scan(&currentLimit)
	if code := validateAccountSettings(accountWrite{LifetimeMinSeconds: x.LifetimeMinSeconds, LifetimeMaxSeconds: x.LifetimeMaxSeconds, BuildSpacingMinutes: x.BuildSpacingMinutes, BuildSpacingMaxMinutes: x.BuildSpacingMaxMinutes, DesiredServerCount: x.DesiredServerCount, Regions: x.Regions, Sizes: x.Sizes, Images: x.Images}, currentLimit, accountProvider); code != "" {
		writeJSON(w, 400, map[string]string{"error": code})
		return
	}
	if x.LifetimeMinSeconds < 1800 {
		x.LifetimeMinSeconds = 1800
	}
	if x.LifetimeMaxSeconds < x.LifetimeMinSeconds {
		x.LifetimeMaxSeconds = x.LifetimeMinSeconds
	}
	x.LifetimeSeconds = x.LifetimeMinSeconds
	if x.BuildSpacingMinutes < 1 {
		x.BuildSpacingMinutes = 60
	}
	if x.BuildSpacingMaxMinutes < x.BuildSpacingMinutes {
		x.BuildSpacingMaxMinutes = x.BuildSpacingMinutes
	}
	x.IntervalSeconds = 60
	if x.BatchSize < 1 {
		x.BatchSize = 1
	}
	if x.DesiredServerCount < 1 {
		x.DesiredServerCount = 1
	}
	if x.MaxConcurrent < 1 {
		x.MaxConcurrent = 1
	}
	fallbackAnyRegion := true
	if x.FallbackAnyRegion != nil {
		fallbackAnyRegion = *x.FallbackAnyRegion
	}
	if len(x.Regions) > 5 {
		x.Regions = x.Regions[:5]
	}
	if len(x.Sizes) > 3 {
		x.Sizes = x.Sizes[:3]
	}
	if len(x.Images) > 3 {
		x.Images = x.Images[:3]
	}
	regions, _ := json.Marshal(x.Regions)
	sizes, _ := json.Marshal(x.Sizes)
	images, _ := json.Marshal(x.Images)
	var candidateEmail, candidateExternalID string
	if x.Token != "" {
		var providerName string
		_ = s.DB.QueryRowContext(r.Context(), `SELECT provider FROM accounts WHERE id=$1`, id).Scan(&providerName)
		d, validateErr := s.validateReplacementToken(r.Context(), id, providerName, x.Token, x.NetworkMode, x.ProxyID)
		if validateErr != nil {
			writeJSON(w, 422, map[string]string{"error": "replacement_token_validation_failed", "detail": validateErr.Error()})
			return
		}
		candidateEmail, candidateExternalID = d.Email, d.ID
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "deployment-admission:"+id); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.NetworkMode == "" {
		x.NetworkMode = "direct"
	}
	if x.NetworkMode != "direct" && x.NetworkMode != "proxy_required" {
		writeJSON(w, 400, map[string]string{"error": "invalid_network_mode"})
		return
	}
	if x.NetworkMode == "proxy_required" {
		var status string
		if x.ProxyID == "" || tx.QueryRowContext(r.Context(), `SELECT status FROM proxies WHERE id=$1`, x.ProxyID).Scan(&status) != nil || status != "healthy" {
			writeJSON(w, 409, map[string]string{"error": "proxy_unavailable", "detail": "Selected proxy is not currently usable"})
			return
		}
	}
	beforeRules, oldDesired, err := app.AccountBuildRulesTx(r.Context(), tx, id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE accounts SET name=$2,preferred_regions=$3,preferred_region=COALESCE(NULLIF($11,''),preferred_region),preferred_sizes=$4,preferred_images=$5,preferred_image=COALESCE(NULLIF($6,''),preferred_image),server_lifetime_seconds=$7,server_lifetime_min_seconds=$7,server_lifetime_max_seconds=$8,auto_interval_seconds=$9,auto_batch_size=$10,auto_max_concurrent=$12,desired_server_count=$13,fallback_any_region=$14,build_spacing_minutes=$15,build_spacing_max_minutes=$16,next_build_at=NULL,updated_at=now() WHERE id=$1`, id, x.Name, regions, sizes, images, func() string {
		if len(x.Images) > 0 {
			return x.Images[0]
		}
		return x.Image
	}(), x.LifetimeMinSeconds, x.LifetimeMaxSeconds, x.IntervalSeconds, x.BatchSize, func() string {
		if len(x.Regions) > 0 {
			return x.Regions[0]
		}
		return ""
	}(), x.MaxConcurrent, x.DesiredServerCount, fallbackAnyRegion, x.BuildSpacingMinutes, x.BuildSpacingMaxMinutes)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if x.NetworkMode == "proxy_required" {
		collision, _ := networkIdentityCollision(r.Context(), tx, id, x.ProxyID)
		if collision {
			writeJSON(w, 409, map[string]string{"error": "network_identity_collision", "detail": "Selected proxy shares an exit IP or subnet with another account"})
			return
		}
	}
	var oldMode, oldProxyID string
	_ = tx.QueryRowContext(r.Context(), `SELECT mode,COALESCE(proxy_id::text,'') FROM network_profiles WHERE account_id=$1`, id).Scan(&oldMode, &oldProxyID)
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,$2,CASE WHEN $2='proxy_required' THEN NULLIF($3,'')::uuid ELSE NULL END) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id,updated_at=now()`, id, x.NetworkMode, x.ProxyID); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.NetworkMode == "proxy_required" {
		if err = syncAccountProxyPool(r.Context(), tx, id, x.ProxyIDs); err != nil {
			writeJSON(w, 500, map[string]string{"error": "proxy_pool_sync_failed", "detail": err.Error()})
			return
		}
	} else {
		_, _ = tx.ExecContext(r.Context(), `UPDATE account_proxy_pool SET enabled=false,updated_at=now() WHERE account_id=$1`, id)
	}
	if oldMode != x.NetworkMode || oldProxyID != x.ProxyID {
		_, _ = tx.ExecContext(r.Context(), `UPDATE account_network_identities SET sticky_session=NULL,fallback_active=false,rotation_started_at=NULL,exit_ip=NULL,subnet_key=NULL,asn=NULL,country=NULL,country_code=NULL,preferred_country=NULL,preferred_country_code=NULL,last_health_ok=false,last_health_at=NULL,updated_at=now() WHERE account_id=$1`, id)
	}
	if err := syncAccountAutomationTx(r.Context(), tx, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": "automation_sync_failed", "detail": err.Error()})
		return
	}
	if x.Token != "" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,''),provider_state='ACTIVE',provider_state_detail=NULL,provider_state_at=now(),provider_error_state=NULL,provider_error_detail=NULL,provider_checked_at=NULL,runtime_status=CASE WHEN deletion_requested_at IS NULL THEN 'ISOLATION_WAIT' ELSE 'DELETE_PENDING' END,runtime_status_detail='credential replaced; provider refresh pending',runtime_status_at=now(),next_build_at=NULL WHERE id=$1`, id, candidateExternalID, candidateEmail); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if x.Token != "" {
		if _, err = tx.ExecContext(r.Context(), "UPDATE account_deletion_jobs SET next_attempt_at=now() WHERE account_id=$1", id); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if err = worker.RearmProviderAuthRetriesTx(r.Context(), tx, id); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if err = app.SaveAccountRuleApplicationTx(r.Context(), tx, id, beforeRules, oldDesired, x.ApplyToExisting); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var previousToken []byte
	var credentialRef, providerForCredential string
	if x.Token != "" {
		_ = s.DB.QueryRowContext(r.Context(), `SELECT secret_ref,provider FROM accounts WHERE id=$1`, id).Scan(&credentialRef, &providerForCredential)
		previousToken, _ = s.Container.Secrets.Get(r.Context(), id, credentialRef)
		if err := s.Container.Secrets.Put(r.Context(), id, credentialRef, providerForCredential+"_credential", []byte(x.Token)); err != nil {
			zeroBytes(previousToken)
			writeJSON(w, 500, map[string]string{"error": "token_update_failed", "detail": err.Error()})
			return
		}
	}
	if err = tx.Commit(); err != nil {
		if x.Token != "" && len(previousToken) > 0 {
			_ = s.Container.Secrets.Put(r.Context(), id, credentialRef, providerForCredential+"_credential", previousToken)
		}
		zeroBytes(previousToken)
		writeJSON(w, 500, errorBody())
		return
	}
	zeroBytes(previousToken)
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout='2s'"); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "deployment-admission:"+id); err != nil {
		w.Header().Set("Retry-After", "3")
		writeJSON(w, 503, map[string]string{"error": "account_busy", "detail": "Account work is settling. Retry deletion shortly."})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "account-mutation:"+id); err != nil {
		w.Header().Set("Retry-After", "3")
		writeJSON(w, 503, map[string]string{"error": "account_busy", "detail": "Account work is settling. Retry deletion shortly."})
		return
	}
	var live int
	if err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM (SELECT provider_resource_id FROM droplets WHERE account_id=$1 AND state<>'DELETED' UNION SELECT provider_resource_id FROM resources WHERE account_id=$1 AND managed AND state<>'deleted') remaining`, id).Scan(&live); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE accounts SET enabled=false,deletion_requested_at=COALESCE(deletion_requested_at,now()),runtime_status='DELETE_PENDING',runtime_status_detail=CASE WHEN deletion_requested_at IS NULL THEN 'deletion requested; cleanup and data purge pending' ELSE runtime_status_detail END,deleted_at=NULL,updated_at=now() WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 200, map[string]any{"state": "ABSENT", "detail": "Account is already absent from this panel."})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE schedules SET enabled=false,lease_until=NULL,updated_at=now() WHERE account_id=$1`, id); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	// Invalidate workflow versions so a stale installer/create runner cannot
	// continue the account after deletion was requested. Unknown provider creates
	// retain their operation journal for read-before-write recovery.
	if _, err = tx.ExecContext(r.Context(), `UPDATE deployments SET state='FAILED',current_step='done',last_error='ACCOUNT_DELETION_REQUESTED',lock_version=lock_version+1,updated_at=now() WHERE account_id=$1 AND state NOT IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE','READY')`, id); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if live > 0 {
		if _, err = tx.ExecContext(r.Context(), `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE account_id=$1 AND state NOT IN ('DELETED','RETIRING','DELETING')`, id); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO account_deletion_jobs(account_id,requested_at) SELECT id,deletion_requested_at FROM accounts WHERE id=$1 ON CONFLICT(account_id) DO UPDATE SET next_attempt_at=now()`, id); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE bulk_lifecycle_scopes SET enabled=false,updated_at=now() WHERE panel_id IN(SELECT id FROM panel_instances WHERE account_id=$1)`, id); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"state": "DELETE_PENDING", "remaining_resources": live, "deletion": s.accountDeletionProgress(r.Context(), id), "detail": "Deletion requested. Managed provider resources must be verified absent before saved account data is purged."})
}

func (s *Server) accountDeletionProgress(ctx context.Context, id string) any {
	var requested, next sql.NullTime
	var attempts, remaining, operations, deployments, keys int
	var detail, providerState string
	var ever bool
	var now time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT a.deletion_requested_at,COALESCE(j.attempts,0),
 (SELECT count(*) FROM (SELECT provider_resource_id FROM droplets WHERE account_id=a.id AND state<>'DELETED' UNION SELECT provider_resource_id FROM resources WHERE account_id=a.id AND managed AND state<>'deleted') remaining),
 COALESCE(NULLIF(j.last_error,''),a.runtime_status_detail,'Waiting for worker'),a.provider_state,
 j.next_attempt_at,now(),
 (SELECT count(*) FROM operations WHERE account_id=a.id AND state NOT IN ('succeeded','failed')),
 (SELECT count(*) FROM deployments WHERE account_id=a.id AND state NOT IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE','READY')),
 (SELECT count(*) FROM account_deletion_keys WHERE account_id=a.id AND state<>'SUCCEEDED'),
 EXISTS(SELECT 1 FROM droplets WHERE account_id=a.id) OR EXISTS(SELECT 1 FROM resources WHERE account_id=a.id) OR EXISTS(SELECT 1 FROM deployments WHERE account_id=a.id) OR EXISTS(SELECT 1 FROM operations WHERE account_id=a.id)
 FROM accounts a LEFT JOIN account_deletion_jobs j ON j.account_id=a.id WHERE a.id=$1`, id).Scan(&requested, &attempts, &remaining, &detail, &providerState, &next, &now, &operations, &deployments, &keys, &ever)
	if err != nil || !requested.Valid {
		return nil
	}
	phase := "VERIFYING"
	settleUntil := requested.Time.Add(2 * time.Minute)
	settling := ever && now.Before(settleUntil)
	switch {
	case remaining > 0 || operations > 0 || deployments > 0 || keys > 0:
		phase = "CLEANUP"
	case settling:
		phase = "SETTLING"
		detail = "Safety wait for earlier provider requests to settle. Cleanup continues automatically; no extra click is needed."
	case attempts == 0:
		phase = "QUEUED"
	}
	if phase == "VERIFYING" && strings.Contains(detail, "waiting for in-flight") {
		detail = "Safety wait completed. Final provider verification is scheduled."
	}
	blocked := providerState == "LOCKED" || providerState == "TOKEN_INVALID" || providerState == "PERMISSION_DENIED" || providerState == "BILLING_BLOCKED"
	if strings.HasPrefix(detail, "Deletion blocked:") {
		phase = "BLOCKED"
	}
	out := map[string]any{"requested_at": requested.Time, "attempts": attempts, "remaining_servers": remaining, "detail": detail, "provider_state": providerState,
		"phase": phase, "server_now": now, "pending_operations": operations, "pending_deployments": deployments, "pending_keys": keys,
		"local_purge_allowed": blocked && !now.Before(settleUntil)}
	if next.Valid {
		out["next_attempt_at"] = next.Time
	}
	if settling {
		out["settle_until"] = settleUntil
	}
	return out
}

// purgeAccount only accepts explicit acknowledgement of unverified cloud
// resources. It is intentionally separate from ordinary provider deletion.
func (s *Server) purgeAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var req struct {
		AccountID     string `json:"account_id"`
		ProviderState string `json:"expected_provider_state"`
		Remaining     *int   `json:"expected_remaining_servers"`
		Acknowledge   bool   `json:"acknowledge_cloud_resources_unverified"`
	}
	id := r.PathValue("id")
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || req.AccountID != id || req.Remaining == nil || *req.Remaining < 0 || !req.Acknowledge {
		writeJSON(w, 400, map[string]string{"error": "explicit_local_purge_acknowledgement_required"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err := app.PurgeSavedAccount(ctx, s.DB, id, req.ProviderState, *req.Remaining)
	var pgerr *pq.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &pgerr) && (pgerr.Code == "55P03" || pgerr.Code == "57014")) {
		w.Header().Set("Retry-After", "5")
		writeJSON(w, 409, map[string]string{"error": "Account cleanup is busy. No partial deletion was committed. Refresh and retry in a few seconds.", "code": "account_cleanup_busy"})
		return
	}
	if errors.Is(err, app.ErrAccountPurgeConflict) {
		writeJSON(w, 409, map[string]string{"error": "Account changed or native cleanup is still in progress. Refresh and review the remaining cloud resources before retrying."})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"state": "PURGED", "provider_deletion_verified": false, "detail": "Account and associated saved data deleted. Cloud resource deletion could not be verified; check the provider console."})
}
