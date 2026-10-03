package cleanup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

type Service struct {
	DB       *sql.DB
	Runtimes *sanaei.RuntimeManager
}

func (s Service) RunOne(parent context.Context) (err error) {
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Share the established client executor lock; cleanup and CREATE/UPDATE/DELETE
	// must never race, even across separate worker processes.
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(628341902731)").Scan(&locked); err != nil || !locked {
		if err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(c, "SELECT pg_advisory_unlock(628341902731)"); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	var job, panel string
	err = s.DB.QueryRowContext(ctx, `SELECT t.job_id::text,t.panel_id::text FROM panel_cleanup_targets t JOIN panel_cleanup_jobs j ON j.id=t.job_id WHERE j.state IN('QUEUED','RUNNING') AND t.state IN('PENDING','RUNNING') ORDER BY j.created_at,t.panel_id LIMIT 1`).Scan(&job, &panel)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.DB.ExecContext(ctx, `UPDATE panel_cleanup_jobs j SET state='SUCCEEDED',completed_at=now() WHERE state IN('QUEUED','RUNNING') AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets t WHERE t.job_id=j.id AND t.state<>'SUCCEEDED')`)
		return err
	}
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			tx, e := s.DB.BeginTx(c, nil)
			if e != nil {
				return
			}
			defer tx.Rollback()
			if _, e = tx.ExecContext(c, "UPDATE panel_cleanup_targets SET state='FAILED',last_error='Fresh verification required; retry resumes the saved scope',updated_at=now() WHERE job_id=$1 AND panel_id=$2", job, panel); e != nil {
				return
			}
			if _, e = tx.ExecContext(c, "UPDATE panel_cleanup_jobs SET state='PAUSED' WHERE id=$1", job); e != nil {
				return
			}
			_ = tx.Commit()
		}
	}()
	return sanaei.WithConfigLock(ctx, s.DB, panel, func(ctx context.Context) error {
		rt, e := s.Runtimes.Acquire(ctx, panel)
		if e != nil {
			return e
		}
		return rt.WithMutation(ctx, func(ctx context.Context) error { return s.execute(ctx, job, panel, rt.Session.Exec) })
	})
}
func (s Service) execute(ctx context.Context, job, panel string, exec sanaei.SessionExecutor) error {
	var frozen bool
	if err := s.DB.QueryRowContext(ctx, `SELECT NOT g.enabled AND (NOT m.enabled OR m.kill_switch) AND (NOT b.enabled OR b.kill_switch)
 AND NOT EXISTS(SELECT 1 FROM bulk_lifecycle_scopes WHERE enabled)
 FROM global_config_policies g CROSS JOIN client_mutation_execution_gate m CROSS JOIN bulk_client_execution_gate b WHERE g.policy_key='reality'`).Scan(&frozen); err != nil {
		return err
	}
	if !frozen {
		return errors.New("creation freeze changed; cleanup stopped")
	}
	observed, err := Observe(ctx, exec)
	if err != nil {
		return err
	}
	var planned bool
	if err = s.DB.QueryRowContext(ctx, "SELECT planned FROM panel_cleanup_targets WHERE job_id=$1 AND panel_id=$2", job, panel).Scan(&planned); err != nil {
		return err
	}
	if !planned {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for email, id := range observed.Clients {
			if _, err = tx.ExecContext(ctx, "INSERT INTO panel_cleanup_clients(job_id,panel_id,email,client_id) VALUES($1,$2,$3,$4)", job, panel, email, id); err != nil {
				return err
			}
		}
		for id, identity := range observed.Inbounds {
			if _, err = tx.ExecContext(ctx, "INSERT INTO panel_cleanup_inbounds(job_id,panel_id,inbound_id,identity) VALUES($1,$2,$3,$4)", job, panel, id, identity); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_targets SET planned=true,state='RUNNING',updated_at=now() WHERE job_id=$1 AND panel_id=$2", job, panel); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_jobs SET state='RUNNING' WHERE id=$1", job); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	clients, inbounds, err := s.members(ctx, job, panel)
	if err != nil {
		return err
	}
	if err = validateObserved(observed, clients, inbounds); err != nil {
		return err
	}
	// Reconcile a crash after POST before deciding whether another mutation is needed.
	if err = s.reconcile(ctx, job, panel, observed); err != nil {
		return err
	}
	if len(observed.Clients) == 0 && len(observed.Inbounds) == 0 {
		return s.complete(ctx, job, panel)
	}
	if _, err = s.DB.ExecContext(ctx, "UPDATE panel_cleanup_targets SET attempts=attempts+1,state='RUNNING',updated_at=now() WHERE job_id=$1 AND panel_id=$2", job, panel); err != nil {
		return err
	}
	after, err := mutationChunk(ctx, exec, observed)
	if err != nil {
		return err
	}
	if err = validateObserved(after, clients, inbounds); err != nil {
		return err
	}
	if err = s.reconcile(ctx, job, panel, after); err != nil {
		return err
	}
	if len(after.Clients) == 0 && len(after.Inbounds) == 0 {
		return s.complete(ctx, job, panel)
	}
	return nil
}
func (s Service) members(ctx context.Context, job, panel string) ([]clientPlan, []inboundPlan, error) {
	var cs []clientPlan
	var ins []inboundPlan
	rows, err := s.DB.QueryContext(ctx, "SELECT email,client_id,confirmed_absent FROM panel_cleanup_clients WHERE job_id=$1 AND panel_id=$2", job, panel)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var c clientPlan
		if err = rows.Scan(&c.Email, &c.ID, &c.Absent); err != nil {
			rows.Close()
			return nil, nil, err
		}
		cs = append(cs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = s.DB.QueryContext(ctx, "SELECT inbound_id,identity,confirmed_absent FROM panel_cleanup_inbounds WHERE job_id=$1 AND panel_id=$2", job, panel)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var in inboundPlan
		if err = rows.Scan(&in.ID, &in.Identity, &in.Absent); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ins = append(ins, in)
	}
	err = rows.Err()
	rows.Close()
	return cs, ins, err
}
func (s Service) reconcile(ctx context.Context, job, panel string, o Observed) error {
	emails := []string{}
	ids := []int64{}
	for e := range o.Clients {
		emails = append(emails, e)
	}
	for id := range o.Inbounds {
		ids = append(ids, id)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_clients SET confirmed_absent=true WHERE job_id=$1 AND panel_id=$2 AND NOT(email=ANY($3::text[]))", job, panel, pq.Array(emails)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_inbounds SET confirmed_absent=true WHERE job_id=$1 AND panel_id=$2 AND NOT(inbound_id=ANY($3::bigint[]))", job, panel, pq.Array(ids)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s Service) complete(ctx context.Context, job, panel string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM output_config_snapshots WHERE panel_id=$1",
		"DELETE FROM panel_client_routes WHERE panel_id=$1",
		"DELETE FROM inbound_export_metadata WHERE panel_id=$1",
		"DELETE FROM inbound_structural_snapshots WHERE panel_id=$1",
		"UPDATE panel_inbound_inventory SET present=false WHERE panel_id=$1",
		"UPDATE client_mutation_jobs SET state='OBSOLETE',last_error='admin deleted all clients and inbounds',completed_at=now() WHERE panel_id=$1 AND state IN('PENDING','RUNNING')",
		"UPDATE bulk_user_ownership o SET state='DELETED',deleted_at=now() FROM bulk_user_generations g WHERE o.generation_id=g.id AND g.panel_id=$1 AND o.state<>'DELETED'",
	} {
		if _, err = tx.ExecContext(ctx, q, panel); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_targets SET state='SUCCEEDED',last_error='',updated_at=now() WHERE job_id=$1 AND panel_id=$2", job, panel); err != nil {
		return err
	}
	return tx.Commit()
}
