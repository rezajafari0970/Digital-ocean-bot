package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func recoverBulk(jobID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.Close()
	j := clientops.Journal{DB: a.DB}
	if err = j.FailCloseGate(ctx); err != nil {
		return err
	}
	defer func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		if e := j.FailCloseGate(c); e != nil {
			log.Printf("CRITICAL gate close: %v", e)
		}
	}()
	job, err := j.Get(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Kind != clientops.KindBulkCreate {
		return fmt.Errorf("not a bulk job")
	}
	if job.State == clientops.StateRunning {
		return fmt.Errorf("job still RUNNING: wait for worker reconciliation before recovery")
	}
	var p clientops.BulkPayload
	if err = json.Unmarshal(job.Payload, &p); err != nil {
		return err
	}
	var purpose string
	if err = a.DB.QueryRowContext(ctx, `SELECT purpose FROM bulk_user_generations WHERE id=$1`, p.GenerationID).Scan(&purpose); err != nil {
		return err
	}
	if purpose != "CANARY" {
		return fmt.Errorf("recovery utility only accepts CANARY generations")
	}
	var other int
	if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING') AND id<>$3::uuid AND idempotency_key NOT LIKE $4`, job.PanelID, job.InboundID, job.ID, "bulk-cleanup:"+job.ID+":%").Scan(&other); err != nil {
		return err
	}
	if other > 0 {
		return fmt.Errorf("unrelated pending jobs in scope")
	}
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: 5 * time.Second}
	rt, err := manager.Acquire(ctx, job.PanelID)
	if err != nil {
		return err
	}
	// Existing successful cleanup jobs may have removed some clients already.
	// First reconcile those before deciding which remaining identities exist.
	_, err = a.DB.ExecContext(ctx, `UPDATE bulk_user_ownership o SET state='DELETED',deleted_at=now() FROM client_mutation_jobs m WHERE o.mutation_job_id=$1 AND m.idempotency_key='bulk-cleanup:'||$1||':'||o.client_id AND m.state='SUCCEEDED' AND o.state='DELETE_PENDING'`, job.ID)
	if err != nil {
		return err
	}
	current, err := snapshot(ctx, rt, job.InboundID)
	if err != nil {
		return err
	}
	// Verify the complete expected payload of present clients through both the
	// inbound snapshot and v3 global API, without touching manual identities.
	observed := []sanaei.Client{}
	for _, c := range p.Clients {
		got, present := current[c.ID]
		if !present {
			continue
		}
		a, b := got, c
		a.LimitHWID, b.LimitHWID = 0, 0
		if a != b {
			return fmt.Errorf("identity conflict; specialized reconciliation required")
		}
		global, e := sanaei.GetClientByEmailSession(ctx, rt.Session.Exec, c.Email)
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(global)
		var g struct {
			UUID  string `json:"uuid"`
			Limit int    `json:"limitHwid"`
		}
		if json.Unmarshal(raw, &g) != nil || g.UUID != c.ID || g.Limit != c.LimitHWID {
			return fmt.Errorf("global identity/device conflict")
		}
		observed = append(observed, c)
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_generations SET state='ROLLING_BACK' WHERE id=$1 AND state<>'CLOSED'`, p.GenerationID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE client_mutation_jobs SET state='OBSOLETE',completed_at=now(),next_retry_at=NULL,last_error='canary recovery: creation closed after fresh read' WHERE id=$1 AND state IN ('PENDING','FAILED')`, job.ID); err != nil {
		return err
	}
	for _, c := range p.Clients {
		if _, present := current[c.ID]; !present {
			if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state=CASE WHEN state='PLANNED' THEN 'ABORTED' ELSE 'DELETED' END,deleted_at=now() WHERE mutation_job_id=$1 AND client_id=$2 AND state IN ('PLANNED','ACTIVE','DELETE_PENDING')`, job.ID, c.ID); err != nil {
				return err
			}
			continue
		}
		var state string
		if err = tx.QueryRowContext(ctx, `SELECT state FROM bulk_user_ownership WHERE mutation_job_id=$1 AND client_id=$2 AND email=$3 FOR UPDATE`, job.ID, c.ID, c.Email).Scan(&state); err != nil {
			return err
		}
		if state == "DELETED" || state == "ABORTED" {
			return fmt.Errorf("terminal ownership reappeared; refusing deletion")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,$3,$4,'DELETE',$5,'{}') ON CONFLICT(account_id,idempotency_key) DO NOTHING`, job.AccountID, job.PanelID, job.InboundID, c.ID, "bulk-cleanup:"+job.ID+":"+c.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETE_PENDING' WHERE mutation_job_id=$1 AND client_id=$2`, job.ID, c.ID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("RECOVERY_VERIFIED job=%s observed=%d absent=%d\n", job.ID, len(observed), len(p.Clients)-len(observed))
	rows, err := a.DB.QueryContext(ctx, `SELECT id::text FROM client_mutation_jobs WHERE idempotency_key LIKE $1 ORDER BY created_at,id`, "bulk-cleanup:"+job.ID+":%")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		if _, err = a.DB.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,concurrency=1,updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, job.PanelID, job.InboundID); err != nil {
			return err
		}
		for _, id := range ids {
			if err = waitJob(ctx, j, id); err != nil {
				return err
			}
		}
	}
	if err = j.FailCloseGate(ctx); err != nil {
		return err
	}
	after, err := snapshot(ctx, rt, job.InboundID)
	if err != nil {
		return err
	}
	for _, c := range p.Clients {
		if _, present := after[c.ID]; present {
			return fmt.Errorf("owned client remains after cleanup")
		}
	}
	tx, err = a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETED',deleted_at=now() WHERE mutation_job_id=$1 AND state='DELETE_PENDING'`, job.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_generations SET state='CLOSED',closed_at=now() WHERE id=$1`, p.GenerationID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("BULK_RECOVERY_OK job=%s remaining_owned=0\n", job.ID)
	return nil
}
