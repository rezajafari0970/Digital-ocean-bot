package proberegistry

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrStore   = errors.New("reality probe registry store failed")
	ErrNoProbe = errors.New("no independent healthy reality probe available")
)

type Node struct {
	ID              string
	Name            string
	Host            string
	Port            int
	SSHUser         string
	SSHKeySecretRef string
	Status          string
}

type Result struct {
	Healthy   bool
	LatencyMS int64
	ExitIP    string
	CheckedAt time.Time
}

type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Select(ctx context.Context, targetHost string) (Node, error) {
	if s.DB == nil || targetHost == "" {
		return Node{}, ErrStore
	}
	var n Node
	e := s.DB.QueryRowContext(ctx, `
SELECT id::text,name,host(host),port,ssh_user,ssh_key_secret_ref,status
FROM reality_probe_nodes
WHERE enabled=true AND status='HEALTHY' AND host<>$1::inet
ORDER BY last_checked_at NULLS FIRST,name
LIMIT 1`, targetHost).Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.SSHUser, &n.SSHKeySecretRef, &n.Status)
	if errors.Is(e, sql.ErrNoRows) {
		return Node{}, ErrNoProbe
	}
	if e != nil {
		return Node{}, ErrStore
	}
	return n, nil
}

func (s SQLStore) SaveResult(ctx context.Context, id string, r Result) error {
	if s.DB == nil || id == "" || r.CheckedAt.IsZero() {
		return ErrStore
	}
	status := "DOWN"
	if r.Healthy {
		status = "HEALTHY"
	}
	_, e := s.DB.ExecContext(ctx, `
UPDATE reality_probe_nodes SET
status=$2,
consecutive_successes=CASE WHEN $3 THEN consecutive_successes+1 ELSE 0 END,
consecutive_failures=CASE WHEN $3 THEN 0 ELSE consecutive_failures+1 END,
last_latency_ms=$4,
last_exit_ip=NULLIF($5,'')::inet,
last_checked_at=$6,
last_success_at=CASE WHEN $3 THEN $6 ELSE last_success_at END,
updated_at=now()
WHERE id=$1`, id, status, r.Healthy, r.LatencyMS, r.ExitIP, r.CheckedAt)
	if e != nil {
		return ErrStore
	}
	return nil
}
