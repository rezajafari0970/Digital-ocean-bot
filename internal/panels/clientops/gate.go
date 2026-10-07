package clientops

import (
	"context"
	"database/sql"
	"errors"
)

type ExecutionGate struct {
	Enabled     bool
	KillSwitch  bool
	PanelID     sql.NullString
	InboundID   sql.NullInt64
	Concurrency int
}

func (j Journal) Gate(ctx context.Context) (ExecutionGate, error) {
	var g ExecutionGate
	if j.DB == nil {
		return g, ErrInvalidRequest
	}
	err := j.DB.QueryRowContext(ctx,
		"SELECT enabled,kill_switch,panel_id::text,inbound_id,concurrency FROM client_mutation_execution_gate WHERE singleton=true",
	).Scan(&g.Enabled, &g.KillSwitch, &g.PanelID, &g.InboundID, &g.Concurrency)
	return g, err
}

func (g ExecutionGate) Allows(job Job) bool {
	if !g.Enabled || g.KillSwitch || g.Concurrency != 1 {
		return false
	}
	if g.PanelID.Valid && g.PanelID.String != job.PanelID {
		return false
	}
	if g.InboundID.Valid && g.InboundID.Int64 != job.InboundID {
		return false
	}
	return true
}

var ErrExecutionGated = errors.New("client mutation execution gated")

func (j Journal) FailCloseGate(ctx context.Context) error {
	return j.FailCloseGateWithFailure(ctx, nil)
}

func (j Journal) FailCloseGateWithFailure(ctx context.Context, failure error) error {
	if j.DB == nil {
		return ErrInvalidRequest
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_client_execution_gate SET enabled=false,kill_switch=true,remaining_batches=0,updated_at=now() WHERE singleton`); err != nil {
		return err
	}
	code, panel, job := "EXPLICIT_GATE_CLOSURE", "", ""
	if failure != nil {
		code = "EXECUTOR_INTERNAL_FAILURE"
		var target *ExecutionFailure
		if errors.As(failure, &target) {
			panel, job = target.PanelID, target.JobID
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true,panel_id=NULL,inbound_id=NULL,concurrency=1,updated_at=now(),last_failure_code=$1,last_failure_at=now(),last_failure_panel_id=$2,last_failure_job_id=$3 WHERE singleton`, code, panel, job); err != nil {
		return err
	}
	return tx.Commit()
}
