package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// ReserveBulkDelete atomically journals a bounded, owned-only deletion. Original
// create-job membership is retained for audit and unknown-outcome recovery.
func (j Journal) ReserveBulkDelete(ctx context.Context, account, panel string, inbound int64, p BulkPayload) (string, error) {
	if len(p.Clients) > 100 {
		return "", ErrInvalidRequest
	}
	if err := p.Validate(); err != nil {
		return "", err
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,941))`, fmt.Sprintf("%s:%d", panel, inbound)); err != nil {
		return "", err
	}
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bulk_user_generations g JOIN panel_instances pi ON pi.id=g.panel_id JOIN bulk_client_execution_gate bg ON bg.singleton WHERE g.id=$1 AND g.panel_id=$2 AND g.inbound_id=$3 AND g.state IN ('ACTIVE','ROLLING_BACK') AND pi.account_id=$4 AND bg.enabled AND NOT bg.kill_switch AND bg.panel_id=$2 AND bg.inbound_id=$3 AND bg.remaining_batches>0 AND bg.expires_at>now() AND bg.max_batch_size>=$5) AND NOT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$2 AND inbound_id=$3 AND state IN ('PENDING','RUNNING','FAILED') AND kind IN ('BULK_CREATE','BULK_DELETE'))`, p.GenerationID, panel, inbound, account, len(p.Clients)).Scan(&allowed)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", ErrExecutionGated
	}
	for _, c := range p.Clients {
		var state string
		err = tx.QueryRowContext(ctx, `SELECT o.state FROM bulk_user_ownership o JOIN client_mutation_jobs m ON m.id=o.mutation_job_id WHERE o.generation_id=$1 AND o.client_id=$2 AND o.email=$3 AND m.kind='BULK_CREATE' AND m.panel_id=$4 AND m.inbound_id=$5 FOR UPDATE OF o`, p.GenerationID, c.ID, c.Email, panel, inbound).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrClientConflict
		}
		if err != nil {
			return "", err
		}
		if state != "ACTIVE" {
			return "", ErrClientConflict
		}
	}
	id, err := sanaei.UUIDv4()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	var jobID string
	err = tx.QueryRowContext(ctx, `INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,$3,$4,'BULK_DELETE',$5,$6) RETURNING id::text`, account, panel, inbound, id, "bulk-delete:"+id, raw).Scan(&jobID)
	if err != nil {
		return "", err
	}
	for _, c := range p.Clients {
		if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETE_PENDING' WHERE generation_id=$1 AND client_id=$2`, p.GenerationID, c.ID); err != nil {
			return "", err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_generations SET state='ROLLING_BACK' WHERE id=$1`, p.GenerationID); err != nil {
		return "", err
	}
	return jobID, tx.Commit()
}

// Identity-only observation is deliberate: expiry/quota changes must not prevent
// owned cleanup. Cross-inbound identity sharing or collisions always fail closed.
func bulkDeleteObserved(raws []json.RawMessage, inbound int64, wanted []sanaei.Client) (present, absent []sanaei.Client, err error) {
	ids, emails := map[string]sanaei.Client{}, map[string]string{}
	locations := map[string]int64{}
	found := false
	wantedIDs, wantedEmails := map[string]bool{}, map[string]bool{}
	for _, c := range wanted {
		wantedIDs[c.ID] = true
		wantedEmails[c.Email] = true
	}
	for _, raw := range raws {
		var in struct {
			ID       int64           `json:"id"`
			Settings json.RawMessage `json:"settings"`
		}
		if err = json.Unmarshal(raw, &in); err != nil {
			return
		}
		if in.ID == inbound {
			found = true
		}
		b := in.Settings
		var encoded string
		if json.Unmarshal(b, &encoded) == nil {
			b = []byte(encoded)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &fields) != nil || fields == nil {
			return nil, nil, ErrVerify
		}
		if in.ID == inbound && (len(fields["clients"]) == 0 || string(fields["clients"]) == "null") {
			return nil, nil, ErrVerify
		}
		var settings struct {
			Clients []sanaei.Client `json:"clients"`
		}
		if err = json.Unmarshal(b, &settings); err != nil {
			return
		}
		for _, c := range settings.Clients {
			if !wantedIDs[c.ID] && !wantedEmails[c.Email] {
				continue
			}
			if old, ok := ids[c.ID]; ok && (old.Email != c.Email || locations[c.ID] != in.ID) {
				return nil, nil, ErrClientConflict
			}
			if old, ok := emails[c.Email]; ok && old != c.ID {
				return nil, nil, ErrClientConflict
			}
			ids[c.ID] = c
			emails[c.Email] = c.ID
			locations[c.ID] = in.ID
		}
	}
	if !found {
		return nil, nil, ErrInboundMissing
	}
	for _, c := range wanted {
		if other, ok := emails[c.Email]; ok && other != c.ID {
			return nil, nil, ErrClientConflict
		}
		if got, ok := ids[c.ID]; ok {
			if got.Email != c.Email || locations[c.ID] != inbound {
				return nil, nil, ErrClientConflict
			}
			present = append(present, c)
		} else {
			absent = append(absent, c)
		}
	}
	return
}
func observeBulkDelete(ctx context.Context, rt *sanaei.PanelRuntime, job Job, p BulkPayload) ([]sanaei.Client, []sanaei.Client, error) {
	rt.Session.Invalidate()
	raws, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	runtimePresent, _, err := bulkDeleteObserved(raws, job.InboundID, p.Clients)
	if err != nil {
		return nil, nil, err
	}
	globals, err := globalWanted(ctx, rt, job.InboundID, p.Clients)
	if err != nil {
		return nil, nil, err
	}
	inRuntime := map[string]bool{}
	for _, c := range runtimePresent {
		inRuntime[c.ID] = true
	}
	present, absent := []sanaei.Client{}, []sanaei.Client{}
	for _, c := range p.Clients {
		if _, exists := globals[c.ID]; exists {
			present = append(present, c)
		} else {
			if inRuntime[c.ID] {
				return nil, nil, ErrVerify
			}
			absent = append(absent, c)
		}
	}
	return present, absent, nil
}
func (e Executor) confirmBulkDeleted(ctx context.Context, p BulkPayload, absent []sanaei.Client) error {
	for _, c := range absent {
		res, err := e.Journal.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETED',deleted_at=COALESCE(deleted_at,now()) WHERE generation_id=$1 AND client_id=$2 AND email=$3 AND state IN ('DELETE_PENDING','DELETED')`, p.GenerationID, c.ID, c.Email)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrClientConflict
		}
	}
	return nil
}
func (e Executor) executeBulkDelete(ctx context.Context, rt *sanaei.PanelRuntime, job Job) error {
	var p BulkPayload
	if json.Unmarshal(job.Payload, &p) != nil {
		return ErrInvalidRequest
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := e.bulkPreflight(ctx, job, p); err != nil {
		return err
	}
	present, absent, err := observeBulkDelete(ctx, rt, job, p)
	if err != nil {
		return err
	}
	if err = e.confirmBulkDeleted(ctx, p, absent); err != nil {
		return err
	}
	if len(present) == 0 {
		return nil
	}
	emails := make([]string, 0, len(present))
	for _, c := range present {
		emails = append(emails, c.Email)
	}
	if err = e.bulkPreflight(ctx, job, p); err != nil {
		return err
	}
	if _, err = e.Journal.DB.ExecContext(ctx, `UPDATE client_mutation_jobs SET result=jsonb_build_object('phase','VERIFY_REQUIRED','submitted',$2::int),updated_at=now() WHERE id=$1 AND state='RUNNING'`, job.ID, len(emails)); err != nil {
		return err
	}
	release, err := e.bulkPostFence(ctx, job, p)
	if err != nil {
		return err
	}
	result, postErr := sanaei.BulkDeleteClientsSession(ctx, rt.Session.Exec, emails)
	release()
	verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	remaining, absent, verifyErr := observeBulkDelete(verifyCtx, rt, job, p)
	if verifyErr == nil {
		if err = e.confirmBulkDeleted(verifyCtx, p, absent); err != nil {
			return err
		}
	}
	report := map[string]any{"phase": "VERIFIED", "response": result, "absent": len(absent), "remaining": len(remaining)}
	if postErr != nil {
		report["post_error"] = postErr.Error()
	}
	if verifyErr != nil {
		report["phase"] = "VERIFY_REQUIRED"
		report["verify_error"] = verifyErr.Error()
	}
	raw, _ := json.Marshal(report)
	if _, err = e.Journal.DB.ExecContext(verifyCtx, `UPDATE client_mutation_jobs SET result=$2 WHERE id=$1 AND state='RUNNING'`, job.ID, raw); err != nil {
		return err
	}
	if verifyErr != nil {
		return verifyErr
	}
	if len(remaining) == 0 && len(absent) == len(p.Clients) {
		return nil
	}
	if postErr != nil {
		return postErr
	}
	return fmt.Errorf("%w: bulk delete remaining=%d", ErrVerify, len(remaining))
}

// ReconcileBulkDeleteOnly reads before an explicitly requested cleanup recovery.
// It never sends mutation requests or changes the immutable batch membership.
func (e Executor) ReconcileBulkDeleteOnly(ctx context.Context, rt *sanaei.PanelRuntime, job Job) ([]sanaei.Client, error) {
	if job.Kind != KindBulkDelete {
		return nil, ErrInvalidRequest
	}
	var p BulkPayload
	if json.Unmarshal(job.Payload, &p) != nil {
		return nil, ErrInvalidRequest
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	present, absent, err := observeBulkDelete(ctx, rt, job, p)
	if err != nil {
		return nil, err
	}

	if err = e.confirmBulkDeleted(ctx, p, absent); err != nil {
		return nil, err
	}
	return present, nil
}

// CompleteBulkRecovery is a CAS transition reserved for fresh-read recovery.
// Worker completion rules remain unchanged; no RUNNING job can be stolen.
func (j Journal) CompleteBulkRecovery(ctx context.Context, job Job, target State) error {
	if job.State != StatePending && job.State != StateFailed {
		return ErrInvalidRequest
	}
	if !((job.Kind == KindBulkCreate && target == StateObsolete) || (job.Kind == KindBulkDelete && target == StateSucceeded)) {
		return ErrInvalidRequest
	}
	res, err := j.DB.ExecContext(ctx, `UPDATE client_mutation_jobs m SET state=$4,completed_at=now(),next_retry_at=NULL,last_error=last_error||'; completed by explicit fresh-read recovery',updated_at=now(),result=result||jsonb_build_object('phase','RECOVERY_VERIFIED') WHERE m.id=$1 AND m.state=$2 AND m.attempts=$3 AND NOT EXISTS(SELECT 1 FROM client_mutation_execution_gate WHERE enabled OR NOT kill_switch) AND NOT EXISTS(SELECT 1 FROM bulk_client_execution_gate WHERE enabled OR NOT kill_switch) AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(m.payload->'Clients') c LEFT JOIN bulk_user_ownership o ON o.generation_id=(m.payload->>'GenerationID')::uuid AND o.client_id=c->>'id' AND o.email=c->>'email' WHERE o.client_id IS NULL OR (m.kind='BULK_DELETE' AND o.state<>'DELETED') OR (m.kind='BULK_CREATE' AND o.state NOT IN ('ACTIVE','ABORTED')))`, job.ID, string(job.State), job.Attempts, string(target))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidRequest
	}
	return nil
}
