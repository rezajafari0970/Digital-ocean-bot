package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type BulkPayload struct {
	Policy         LifecyclePolicy `json:",omitempty"`
	Lifecycle      bool
	CleanupReasons map[string]string `json:",omitempty"`
	GenerationID   string
	TargetUsers    int
	Clients        []sanaei.Client
}

func (p BulkPayload) Validate() error {
	if p.TargetUsers < 1 || p.TargetUsers > 10000 || p.GenerationID == "" || len(p.Clients) == 0 || len(p.Clients) > 250 {
		return ErrInvalidRequest
	}
	ids, emails := map[string]bool{}, map[string]bool{}
	for _, c := range p.Clients {
		if c.ID == "" || c.Email == "" || !c.Enable || c.TotalGB < 0 || c.ExpiryTime < 0 || c.LimitHWID < 0 || ids[c.ID] || emails[c.Email] {
			return ErrInvalidRequest
		}
		ids[c.ID], emails[c.Email] = true, true
	}
	return nil
}

// ReserveBulk commits immutable payload and all ownership rows together. The
// scoped lock also fences competing planners using another process/runtime.
func (j Journal) ReserveBulk(ctx context.Context, accountID, panelID string, inboundID int64, p BulkPayload) (string, bool, error) {
	if err := p.Validate(); err != nil {
		return "", false, err
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,941))`, fmt.Sprintf("%s:%d", panelID, inboundID)); err != nil {
		return "", false, err
	}
	var blocked bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND kind IN ('BULK_CREATE','BULK_DELETE') AND state IN ('PENDING','RUNNING','FAILED')) OR EXISTS(SELECT 1 FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state IN ('PLANNED','DELETE_PENDING'))`, panelID, inboundID).Scan(&blocked); err != nil || blocked {
		return "", false, err
	}
	var marker string
	err = tx.QueryRowContext(ctx, `SELECT g.marker FROM bulk_user_generations g JOIN panel_instances p ON p.id=g.panel_id JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id JOIN bulk_client_execution_gate bg ON bg.singleton WHERE g.id=$1 AND g.panel_id=$2 AND g.inbound_id=$3 AND g.state='ACTIVE' AND p.account_id=$4 AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND bg.enabled AND NOT bg.kill_switch AND bg.panel_id=p.id AND bg.inbound_id=g.inbound_id AND bg.expires_at>now() AND bg.remaining_batches>0 AND bg.max_batch_size >= $5 FOR SHARE OF g,p,bg`, p.GenerationID, panelID, inboundID, accountID, len(p.Clients)).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrExecutionGated
	}
	if err != nil {
		return "", false, err
	}
	for _, c := range p.Clients {
		if !validBulkEmail(marker, c) {
			return "", false, ErrClientConflict
		}
	}
	batchID, err := sanaei.UUIDv4()
	if err != nil {
		return "", false, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", false, err
	}
	var jobID string
	err = tx.QueryRowContext(ctx, `INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,$3,$4,'BULK_CREATE',$5,$6) RETURNING id::text`, accountID, panelID, inboundID, batchID, "bulk:"+batchID, raw).Scan(&jobID)
	if err != nil {
		return "", false, err
	}
	for _, c := range p.Clients {
		if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_user_ownership(generation_id,client_id,email,state,mutation_job_id) VALUES($1,$2,$3,'PLANNED',$4)`, p.GenerationID, c.ID, c.Email, jobID); err != nil {
			return "", false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", false, err
	}
	return jobID, true, nil
}

func validBulkEmail(marker string, c sanaei.Client) bool {
	short := ""
	for _, r := range c.ID {
		if r != '-' {
			short += string(r)
		}
	}
	if len(short) > 8 {
		short = short[:8]
	}
	return c.Email == "u-"+marker+"-"+short
}

// bulkObserved validates the entire snapshot before deciding that any client is
// missing. Identity collisions in other inbounds fail closed.
func bulkObserved(raws []json.RawMessage, inboundID int64, wanted []sanaei.Client) ([]sanaei.Client, []sanaei.Client, error) {
	wantedIDs, wantedEmails := map[string]bool{}, map[string]bool{}
	for _, c := range wanted {
		wantedIDs[c.ID] = true
		wantedEmails[c.Email] = true
	}
	found := false
	ids := map[string]sanaei.Client{}
	emails := map[string]string{}
	locations := map[string]int64{}
	for _, raw := range raws {
		var in struct {
			ID       int64           `json:"id"`
			Enable   bool            `json:"enable"`
			Settings json.RawMessage `json:"settings"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		if in.ID == inboundID {
			if !in.Enable {
				return nil, nil, ErrInboundMissing
			}
			found = true
		}
		b := in.Settings
		var encoded string
		if json.Unmarshal(b, &encoded) == nil {
			b = []byte(encoded)
		}
		var settings struct {
			Clients []sanaei.Client `json:"clients"`
		}
		if err := json.Unmarshal(b, &settings); err != nil {
			return nil, nil, err
		}
		for _, c := range settings.Clients {
			if !wantedIDs[c.ID] && !wantedEmails[c.Email] {
				continue
			}
			if old, ok := emails[c.Email]; ok && old != c.ID {
				return nil, nil, ErrClientConflict
			}
			emails[c.Email] = c.ID
			if old, ok := ids[c.ID]; ok && (old.Email != c.Email || locations[c.ID] != in.ID) {
				return nil, nil, ErrClientConflict
			}
			ids[c.ID] = c
			locations[c.ID] = in.ID
		}
	}
	if !found {
		return nil, nil, ErrInboundMissing
	}
	var observed, missing []sanaei.Client
	for _, want := range wanted {
		got, exists := ids[want.ID]
		if other, ok := emails[want.Email]; ok && other != want.ID {
			return observed, missing, ErrClientConflict
		}
		if !exists {
			missing = append(missing, want)
			continue
		}
		// HWID is stored in the v3 global client record, not the Xray settings.
		a, b := got, want
		a.LimitHWID, b.LimitHWID = 0, 0
		if locations[want.ID] != inboundID || !sameClient(a, b) {
			return observed, missing, ErrClientConflict
		}
		observed = append(observed, want)
	}
	return observed, missing, nil
}

func observeBulk(ctx context.Context, rt *sanaei.PanelRuntime, job Job, p BulkPayload) ([]sanaei.Client, []sanaei.Client, error) {
	rt.Session.Invalidate()
	raws, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	observed, missing, err := bulkObserved(raws, job.InboundID, p.Clients)
	if err == nil {
		for _, raw := range raws {
			var in struct {
				ID       int64           `json:"id"`
				Settings json.RawMessage `json:"settings"`
			}
			if json.Unmarshal(raw, &in) != nil || in.ID != job.InboundID {
				continue
			}
			b := in.Settings
			var encoded string
			if json.Unmarshal(b, &encoded) == nil {
				b = []byte(encoded)
			}
			var settings struct {
				Clients []sanaei.Client `json:"clients"`
			}
			if json.Unmarshal(b, &settings) != nil {
				return nil, nil, ErrVerify
			}
			if len(settings.Clients)+len(missing) > p.TargetUsers {
				return nil, nil, fmt.Errorf("%w: capacity changed after planning", ErrClientConflict)
			}
		}
	}

	if err != nil {
		return nil, nil, err
	}
	confirmed := make([]sanaei.Client, 0, len(observed))
	globals, err := globalWanted(ctx, rt, job.InboundID, p.Clients)
	if err != nil {
		return confirmed, missing, err
	}
	for _, c := range missing {
		if _, exists := globals[c.ID]; exists {
			return confirmed, missing, ErrVerify
		}
	}
	for _, c := range observed {
		g, ok := globals[c.ID]
		if !ok || g.UUID != c.ID || g.Email != c.Email || g.TotalGB != c.TotalGB || g.ExpiryTime != c.ExpiryTime || g.LimitHWID != c.LimitHWID || g.Enable != c.Enable {
			return confirmed, missing, ErrClientConflict
		}
		confirmed = append(confirmed, c)
	}
	return confirmed, missing, nil
}

func (e Executor) confirmBulk(ctx context.Context, job Job, clients []sanaei.Client) error {
	for _, c := range clients {
		res, err := e.Journal.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='ACTIVE',recovery_checks=0,last_recovery_check_at=now() WHERE mutation_job_id=$1 AND client_id=$2 AND email=$3 AND state IN ('PLANNED','ACTIVE')`, job.ID, c.ID, c.Email)
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

func (e Executor) bulkPreflight(ctx context.Context, job Job, p BulkPayload) error {
	if p.Lifecycle {
		return e.lifecyclePreflight(ctx, job, p.GenerationID)
	}
	var allowed bool
	err := e.Journal.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM client_mutation_jobs m JOIN client_mutation_execution_gate g ON g.singleton JOIN bulk_client_execution_gate bg ON bg.singleton JOIN panel_instances pi ON pi.id=m.panel_id JOIN droplets d ON d.id=pi.droplet_id JOIN deployments dep ON dep.droplet_id=d.id JOIN accounts a ON a.id=pi.account_id JOIN bulk_user_generations gen ON gen.id=$3 WHERE m.id=$1 AND m.state='RUNNING' AND m.attempts=$2 AND (gen.state='ACTIVE' OR (m.kind='BULK_DELETE' AND gen.state='ROLLING_BACK')) AND gen.panel_id=m.panel_id AND gen.inbound_id=m.inbound_id AND pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=m.panel_id) AND (g.inbound_id IS NULL OR g.inbound_id=m.inbound_id) AND bg.enabled AND NOT bg.kill_switch AND bg.panel_id=m.panel_id AND bg.inbound_id=m.inbound_id AND bg.expires_at>now() AND bg.max_batch_size >= $4)`, job.ID, job.Attempts, p.GenerationID, len(p.Clients)).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrExecutionGated
	}
	var count int
	ids := make([]string, 0, len(p.Clients))
	for _, c := range p.Clients {
		ids = append(ids, c.ID)
	}
	err = e.Journal.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_user_ownership WHERE generation_id=$2 AND client_id=ANY($3) AND ((mutation_job_id=$1 AND state IN ('PLANNED','ACTIVE') AND $4='BULK_CREATE') OR ($4='BULK_DELETE' AND state IN ('DELETE_PENDING','DELETED')))`, job.ID, p.GenerationID, pq.Array(ids), string(job.Kind)).Scan(&count)
	if err != nil {
		return err
	}
	if count != len(p.Clients) {
		return ErrClientConflict
	}
	return nil
}

func (e Executor) executeBulk(ctx context.Context, rt *sanaei.PanelRuntime, job Job) error {
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
	observed, missing, err := observeBulk(ctx, rt, job, p)
	if ce := e.confirmBulk(ctx, job, observed); ce != nil {
		return ce
	}
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	for _, c := range missing {
		if c.ExpiryTime > 0 && c.ExpiryTime <= time.Now().Add(10*time.Second).UnixMilli() {
			return fmt.Errorf("%w: planned lifetime elapsed", ErrClientConflict)
		}
	}
	if err = e.bulkPreflight(ctx, job, p); err != nil {
		return err
	}
	// Persist intent before the only POST. On a crash the same payload is read
	// back; no new client identities can be minted by recovery.
	if _, err = e.Journal.DB.ExecContext(ctx, `UPDATE client_mutation_jobs SET result=jsonb_build_object('phase','VERIFY_REQUIRED','submitted',$2::int),updated_at=now() WHERE id=$1 AND state='RUNNING'`, job.ID, len(missing)); err != nil {
		return err
	}
	release, err := e.bulkPostFence(ctx, job, p)
	if err != nil {
		return err
	}
	result, postErr := sanaei.BulkCreateClientsSession(ctx, rt.Session.Exec, int(job.InboundID), missing)
	release()
	// Even a lost POST response must be followed by a fresh observation. Use a
	// separate bounded read context if the request context has just timed out.
	verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	observed, remaining, verifyErr := observeBulk(verifyCtx, rt, job, p)
	if err = e.confirmBulk(verifyCtx, job, observed); err != nil {
		verifyErr = err
	}
	report := map[string]any{"phase": "VERIFIED", "response": result, "observed": len(observed), "missing": len(remaining)}
	if postErr != nil {
		report["post_error"] = postErr.Error()
	}
	if verifyErr != nil {
		report["verify_error"] = verifyErr.Error()
		report["phase"] = "VERIFY_REQUIRED"
	}
	b, _ := json.Marshal(report)
	reportCtx, reportCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer reportCancel()
	if _, err = e.Journal.DB.ExecContext(reportCtx, `UPDATE client_mutation_jobs SET result=$2 WHERE id=$1 AND state='RUNNING'`, job.ID, b); err != nil {
		return err
	}
	if verifyErr != nil {
		return verifyErr
	}
	if len(remaining) == 0 && len(observed) == len(p.Clients) {
		return nil
	}
	if len(result.Skipped) > 0 {
		for _, c := range remaining {
			for _, skipped := range result.Skipped {
				if skipped.Email == c.Email {
					return fmt.Errorf("%w: bulk skipped an absent planned identity", ErrClientConflict)
				}
			}
		}
	}
	if postErr != nil {
		return postErr
	}
	return fmt.Errorf("%w: bulk observed=%d missing=%d skipped=%d", ErrVerify, len(observed), len(remaining), len(result.Skipped))
}

// The row locks define the start of an in-flight POST. A gate close or lifecycle
// change waits for this bounded request; once closing commits no later POST can
// pass this fence. No pool connection is held across readback verification.
func (e Executor) bulkPostFence(ctx context.Context, job Job, p BulkPayload) (func(), error) {
	if p.Lifecycle {
		return e.lifecycleFence(ctx, job, p.GenerationID, true)
	}
	tx, err := e.Journal.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	var id string
	err = tx.QueryRowContext(ctx, `SELECT m.id::text FROM client_mutation_jobs m JOIN client_mutation_execution_gate g ON g.singleton JOIN bulk_client_execution_gate bg ON bg.singleton JOIN panel_instances pi ON pi.id=m.panel_id JOIN droplets d ON d.id=pi.droplet_id JOIN deployments dep ON dep.droplet_id=d.id JOIN accounts a ON a.id=pi.account_id JOIN bulk_user_generations gen ON gen.id=$3 WHERE m.id=$1 AND m.state='RUNNING' AND m.attempts=$2 AND (gen.state='ACTIVE' OR (m.kind='BULK_DELETE' AND gen.state='ROLLING_BACK')) AND pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=m.panel_id) AND (g.inbound_id IS NULL OR g.inbound_id=m.inbound_id) AND bg.enabled AND NOT bg.kill_switch AND bg.panel_id=m.panel_id AND bg.inbound_id=m.inbound_id AND bg.expires_at>now() FOR SHARE OF g,bg,pi,d,a,dep,gen`, job.ID, job.Attempts, p.GenerationID).Scan(&id)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrExecutionGated
		}
		return nil, err
	}
	return func() { tx.Rollback() }, nil
}

// ReconcileBulkOnly is safe with closed gates: it only reads the panel and
// confirms matching ownership. It never sends POST or invents new identities.
func (e Executor) ReconcileBulkOnly(ctx context.Context, rt *sanaei.PanelRuntime, job Job) ([]sanaei.Client, []sanaei.Client, error) {
	if job.Kind != KindBulkCreate {
		return nil, nil, ErrInvalidRequest
	}
	var p BulkPayload
	if json.Unmarshal(job.Payload, &p) != nil {
		return nil, nil, ErrInvalidRequest
	}
	if err := p.Validate(); err != nil {
		return nil, nil, err
	}
	observed, missing, err := observeBulk(ctx, rt, job, p)
	if ce := e.confirmBulk(ctx, job, observed); ce != nil {
		return nil, nil, ce
	}
	return observed, missing, err
}
