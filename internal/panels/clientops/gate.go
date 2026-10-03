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
	if j.DB == nil {
		return ErrInvalidRequest
	}
	_, err := j.DB.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true,panel_id=NULL,inbound_id=NULL,concurrency=1,updated_at=now() WHERE singleton=true`)
	return err
}
