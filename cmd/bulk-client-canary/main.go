package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/usercapacity"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func snapshot(ctx context.Context, rt *sanaei.PanelRuntime, inbound int64) (map[string]sanaei.Client, error) {
	rt.Session.Invalidate()
	raw, found, err := rt.Session.RawInbound(ctx, inbound)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("inbound missing")
	}
	var in struct {
		Settings json.RawMessage `json:"settings"`
	}
	if err = json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	b := in.Settings
	var encoded string
	if json.Unmarshal(b, &encoded) == nil {
		b = []byte(encoded)
	}
	var settings struct {
		Clients []sanaei.Client `json:"clients"`
	}
	if err = json.Unmarshal(b, &settings); err != nil {
		return nil, err
	}
	out := map[string]sanaei.Client{}
	for _, c := range settings.Clients {
		out[c.ID] = c
	}
	return out, nil
}
func waitJob(ctx context.Context, j clientops.Journal, id string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := j.Get(ctx, id)
		if err != nil {
			return err
		}
		switch job.State {
		case clientops.StateSucceeded:
			return nil
		case clientops.StateFailed, clientops.StateObsolete:
			return fmt.Errorf("job=%s state=%s error=%s", id, job.State, job.LastError)
		}
		gate, err := j.Gate(ctx)
		if err != nil {
			return err
		}
		if !gate.Enabled || gate.KillSwitch {
			return fmt.Errorf("executor fail-closed job=%s state=%s error=%s", id, job.State, job.LastError)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func run() error {
	var panel, recoverJob string
	var inbound int64
	var count int
	flag.StringVar(&panel, "panel", "", "exact panel uuid")
	flag.Int64Var(&inbound, "inbound", 0, "exact inbound")
	flag.StringVar(&recoverJob, "recover-job", "", "read-before-write cleanup of an existing bulk job")
	flag.IntVar(&count, "count", 10, "bounded canary size: 10, 25, 50 or 100")
	flag.Parse()
	if recoverJob != "" {
		return recoverBulk(recoverJob)
	}
	if err := validateCanaryCount(count); err != nil {
		return err
	}
	if panel == "" || inbound <= 0 {
		return fmt.Errorf("panel and inbound required")
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, 4*time.Minute)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.Close()
	journal := clientops.Journal{DB: a.DB}
	gate, err := journal.Gate(ctx)
	if err != nil {
		return err
	}
	if gate.Enabled || !gate.KillSwitch {
		return fmt.Errorf("main gate must be closed")
	}
	var policyEnabled bool
	if err = a.DB.QueryRowContext(ctx, `SELECT enabled FROM global_config_policies WHERE policy_key='reality'`).Scan(&policyEnabled); err != nil {
		return err
	}
	if policyEnabled {
		return fmt.Errorf("isolated canary requires current policy disabled")
	}
	var active bool
	if err = a.DB.QueryRowContext(ctx, `SELECT enabled OR NOT kill_switch FROM bulk_client_execution_gate WHERE singleton`).Scan(&active); err != nil {
		return err
	}
	if active {
		return fmt.Errorf("bulk gate must be closed")
	}
	var pending int
	if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING')`, panel, inbound).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return fmt.Errorf("scope has pending jobs")
	}
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: 5 * time.Second}
	rt, err := manager.Acquire(ctx, panel)
	if err != nil {
		return err
	}
	baseline, err := snapshot(ctx, rt, inbound)
	if err != nil {
		return err
	}
	if len(baseline) != 1 {
		return fmt.Errorf("expected one preserved manual client, observed %d", len(baseline))
	}
	rawBase, _ := json.Marshal(baseline)
	baseHash := sha256.Sum256(rawBase)
	fmt.Printf("BASELINE count=%d sha256=%x\n", len(baseline), baseHash)
	defer func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		if e := journal.FailCloseGate(c); e != nil {
			log.Printf("CRITICAL gate close: %v", e)
		}
	}()
	res, err := a.DB.ExecContext(ctx, `UPDATE bulk_client_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,max_batch_size=$3,remaining_batches=1,expires_at=now()+interval '3 minutes',updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, panel, inbound, count)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("gate changed concurrently")
	}
	markerID, err := sanaei.UUIDv4()
	if err != nil {
		return err
	}
	marker := "bulkcanary" + strings.ReplaceAll(markerID, "-", "")
	var gen string
	if err = a.DB.QueryRowContext(ctx, `INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,$2,'CANARY',$3) RETURNING id::text`, panel, inbound, marker).Scan(&gen); err != nil {
		return err
	}
	svc := usercapacity.Service{DB: a.DB, Secrets: a.Container.Secrets}
	// One bounded chunk at the same explicit scoped users/second rate. The durable
	// bucket is shared with policy planning and is never reset or bypassed.
	allowance, err := svc.BulkAllowance(ctx, panel, inbound, count, count)
	if err != nil {
		return err
	}
	for allowance < count {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		allowance, err = svc.BulkAllowance(ctx, panel, inbound, count, count)
		if err != nil {
			return err
		}
	}
	payload := clientops.BulkPayload{GenerationID: gen, TargetUsers: len(baseline) + count}
	for i := 0; i < count; i++ {
		id, e := sanaei.UUIDv4()
		if e != nil {
			return e
		}
		payload.Clients = append(payload.Clients, sanaei.Client{ID: id, Email: "u-" + marker + "-" + strings.ReplaceAll(id, "-", "")[:8], Enable: true, TotalGB: 104857600, ExpiryTime: time.Now().Add(30 * time.Minute).UnixMilli(), LimitHWID: 2, Flow: "xtls-rprx-vision"})
	}
	jobID, planned, err := journal.ReserveBulk(ctx, rt.AccountID, panel, inbound, payload)
	if err != nil {
		return err
	}
	if !planned {
		return fmt.Errorf("scope blocked before planning")
	}
	fmt.Printf("PLANNED job=%s generation=%s clients=%d\n", jobID, gen, count)
	fmt.Printf("RECOVERY: bulk-client-canary -recover-job %s\n", jobID)
	var plannedCount int
	if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_user_ownership WHERE mutation_job_id=$1 AND state='PLANNED'`, jobID).Scan(&plannedCount); err != nil {
		return err
	}
	if plannedCount != count {
		return fmt.Errorf("planned ownership=%d", plannedCount)
	}
	createStarted := time.Now()
	res, err = a.DB.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,concurrency=1,updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, panel, inbound)
	if err != nil {
		return err
	}
	n, _ = res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("main gate changed concurrently")
	}
	if err = waitJob(ctx, journal, jobID); err != nil {
		return err
	}
	if err = journal.FailCloseGate(ctx); err != nil {
		return err
	}
	current, err := snapshot(ctx, rt, inbound)
	if err != nil {
		return err
	}
	if len(current) != len(baseline)+count {
		return fmt.Errorf("created count=%d", len(current))
	}
	for id, c := range baseline {
		if current[id] != c {
			return fmt.Errorf("baseline client changed")
		}
	}
	var owned int
	if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_user_ownership WHERE mutation_job_id=$1 AND state='ACTIVE'`, jobID).Scan(&owned); err != nil {
		return err
	}
	if owned != count {
		return fmt.Errorf("active ownership=%d", owned)
	}
	fmt.Printf("CREATE_VERIFIED active_owned=%d quota=104857600 hwid=2 elapsed_ms=%d\n", count, time.Since(createStarted).Milliseconds())
	outputStarted := time.Now()
	outputOK := false
	for i := 0; i < 20; i++ {
		var output int
		err = a.DB.QueryRowContext(ctx, `SELECT count(DISTINCT o.client_id) FROM bulk_user_ownership o JOIN output_config_snapshots s ON s.panel_id=$2 AND s.uri LIKE '%'||o.client_id||'%' WHERE o.mutation_job_id=$1 AND s.visible_until>now() AND s.last_seen_at>now()-interval '15 seconds'`, jobID, panel).Scan(&output)
		if err != nil {
			return err
		}
		if output == count {
			outputOK = true
			fmt.Printf("OUTPUT_VERIFIED clients=%d elapsed_ms=%d\n", count, time.Since(outputStarted).Milliseconds())
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	cleanupStarted := time.Now()
	// Queue cleanup atomically from exactly this immutable ownership membership.
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range payload.Clients {
		var deleteID string
		err = tx.QueryRowContext(ctx, `INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) SELECT $1,$2,$3,o.client_id,'DELETE','bulk-cleanup:'||$4||':'||o.client_id,'{}'::jsonb FROM bulk_user_ownership o WHERE o.mutation_job_id=$4::uuid AND o.client_id=$5 AND o.email=$6 AND o.state='ACTIVE' RETURNING id::text`, rt.AccountID, panel, inbound, jobID, c.ID, c.Email).Scan(&deleteID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETE_PENDING' WHERE mutation_job_id=$1 AND client_id=$2 AND state='ACTIVE'`, jobID, c.ID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if _, err = a.DB.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, panel, inbound); err != nil {
		return err
	}
	rows, err := a.DB.QueryContext(ctx, `SELECT id::text FROM client_mutation_jobs WHERE idempotency_key LIKE $1 ORDER BY created_at,id`, "bulk-cleanup:"+jobID+":%")
	if err != nil {
		return err
	}
	var deletes []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		deletes = append(deletes, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(deletes) != count {
		return fmt.Errorf("cleanup jobs=%d", len(deletes))
	}
	for _, id := range deletes {
		if err = waitJob(ctx, journal, id); err != nil {
			return err
		}
	}
	if err = journal.FailCloseGate(ctx); err != nil {
		return err
	}
	after, err := snapshot(ctx, rt, inbound)
	if err != nil {
		return err
	}
	rawAfter, _ := json.Marshal(after)
	afterHash := sha256.Sum256(rawAfter)
	if afterHash != baseHash {
		return fmt.Errorf("baseline not restored")
	}
	tx, err = a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETED',deleted_at=now() WHERE mutation_job_id=$1 AND state='DELETE_PENDING'`, jobID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_generations SET state='CLOSED',closed_at=now() WHERE id=$1`, gen); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("CLEANUP_VERIFIED deleted=%d baseline_count=%d sha256=%x elapsed_ms=%d\n", count, len(after), afterHash, time.Since(cleanupStarted).Milliseconds())
	if !outputOK {
		return fmt.Errorf("cleanup succeeded; output visibility acceptance failed")
	}
	fmt.Printf("BULK_CANARY_OK job=%s generation=%s\n", jobID, gen)
	return nil
}

func validateCanaryCount(count int) error {
	if count != 10 && count != 25 && count != 50 && count != 100 {
		return fmt.Errorf("canary count must be 10, 25, 50 or 100; larger stages require acceptance")
	}
	return nil
}
