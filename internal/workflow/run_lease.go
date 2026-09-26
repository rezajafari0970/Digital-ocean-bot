package workflow

import (
	"context"
	"database/sql"
	"errors"
)

var ErrDeploymentBusy = errors.New("deployment already running")

type RunLease interface {
	Acquire(context.Context, string) (func(), error)
}

type PostgresRunLease struct {
	DB *sql.DB
}

func (l PostgresRunLease) Acquire(ctx context.Context, deploymentID string) (func(), error) {
	if l.DB == nil || deploymentID == "" {
		return nil, ErrDeploymentBusy
	}
	conn, err := l.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "deployment:"+deploymentID).Scan(&locked); err != nil {
		conn.Close()
		return nil, err
	}
	if !locked {
		conn.Close()
		return nil, ErrDeploymentBusy
	}
	release := func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "deployment:"+deploymentID)
		_ = conn.Close()
	}
	return release, nil
}
