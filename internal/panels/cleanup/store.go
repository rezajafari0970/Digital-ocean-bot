package cleanup

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lib/pq"
	"time"
)

type Store struct{ DB *sql.DB }
type Result struct {
	PanelID         string `json:"panel_id"`
	Deleted         int    `json:"deleted"`
	DeletedInbounds int    `json:"deleted_inbounds"`
	Error           string `json:"error,omitempty"`
	State           string `json:"state"`
}
type Status struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Total     int       `json:"total"`
	Completed int       `json:"completed"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
	Results   []Result  `json:"results"`
	StartedAt time.Time `json:"started_at"`
}

// Start snapshots scope and closes every automatic creation path atomically.
// A repeated request resumes the same immutable plan.
func (s Store) Start(ctx context.Context, scope []string) (string, error) {
	if scope == nil {
		scope = []string{}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(628341902731)"); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(628341902732)"); err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRowContext(ctx, "SELECT id::text FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED') FOR UPDATE").Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if id == "" {
		if err = tx.QueryRowContext(ctx, "INSERT INTO panel_cleanup_jobs DEFAULT VALUES RETURNING id::text").Scan(&id); err != nil {
			return "", err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO panel_cleanup_targets(job_id,panel_id) SELECT $1,p.id FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id WHERE p.driver='sanaei-3x-ui' AND dr.state<>'DELETED' AND (cardinality($2::uuid[])=0 OR p.id=ANY($2::uuid[]))`, id, pq.Array(scope)); err != nil {
			return "", err
		}
		if len(scope) > 0 {
			var n int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM panel_cleanup_targets WHERE job_id=$1", id).Scan(&n); err != nil || n != len(scope) {
				return "", errors.New("cleanup scope mismatch")
			}
		}
	} else {
		if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_targets SET state='PENDING',last_error='' WHERE job_id=$1 AND state='FAILED'", id); err != nil {
			return "", err
		}
	}
	for _, q := range []string{
		"UPDATE global_config_policies SET enabled=false,updated_at=now() WHERE policy_key='reality'",
		"UPDATE bulk_lifecycle_scopes SET enabled=false WHERE enabled",
		"UPDATE bulk_client_execution_gate SET enabled=false,kill_switch=true,updated_at=now()",
		"UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true,updated_at=now()",
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return "", err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_jobs SET state='QUEUED' WHERE id=$1", id); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM output_config_snapshots WHERE panel_id IN(SELECT panel_id FROM panel_cleanup_targets WHERE job_id=$1)`, id); err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func (s Store) Status(ctx context.Context, id string) (Status, error) {
	st := Status{ID: id, Results: []Result{}}
	if err := s.DB.QueryRowContext(ctx, "SELECT state,created_at FROM panel_cleanup_jobs WHERE id=$1", id).Scan(&st.Status, &st.StartedAt); err != nil {
		return st, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT t.panel_id::text,t.state,t.last_error,
 (SELECT count(*) FROM panel_cleanup_clients c WHERE c.job_id=t.job_id AND c.panel_id=t.panel_id AND c.confirmed_absent),
 (SELECT count(*) FROM panel_cleanup_inbounds i WHERE i.job_id=t.job_id AND i.panel_id=t.panel_id AND i.confirmed_absent)
 FROM panel_cleanup_targets t WHERE t.job_id=$1 ORDER BY t.panel_id`, id)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Result
		if err = rows.Scan(&r.PanelID, &r.State, &r.Error, &r.Deleted, &r.DeletedInbounds); err != nil {
			return st, err
		}
		st.Total++
		if r.State == "SUCCEEDED" {
			st.Succeeded++
			st.Completed++
		}
		if r.State == "FAILED" {
			st.Failed++
			st.Completed++
		}
		st.Results = append(st.Results, r)
	}
	if st.Status == "SUCCEEDED" {
		st.Status = "done"
	} else if st.Status == "CANCELLED" {
		st.Status = "cancelled"
	} else if st.Status == "PAUSED" {
		st.Status = "paused"
	} else {
		st.Status = "running"
	}
	return st, rows.Err()
}

// Current returns the latest durable status so page reloads never lose progress.
func (s Store) Current(ctx context.Context) (Status, error) {
	var id string
	if err := s.DB.QueryRowContext(ctx, "SELECT id::text FROM panel_cleanup_jobs ORDER BY created_at DESC LIMIT 1").Scan(&id); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, id)
}

// Cancel waits for the current mutation to finish before permanently stopping
// this saved scope. It preserves all observations and never claims a deletion succeeded.
func (s Store) Cancel(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, lock := range []int64{628341902731, 628341902732} {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lock); err != nil {
			return err
		}
	}
	var state string
	if err = tx.QueryRowContext(ctx, "SELECT state FROM panel_cleanup_jobs WHERE id=$1 FOR UPDATE", id).Scan(&state); err != nil {
		return err
	}
	if state == "CANCELLED" || state == "SUCCEEDED" {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_cleanup_jobs SET state='CANCELLED',completed_at=now() WHERE id=$1", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id IN(SELECT panel_id FROM panel_cleanup_targets WHERE job_id=$1)", id); err != nil {
		return err
	}
	return tx.Commit()
}
