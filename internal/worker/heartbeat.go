package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Heartbeat struct {
	DB       *sql.DB
	WorkerID string
	Kind     string
}

func (h Heartbeat) Beat(ctx context.Context, metadata map[string]any) error {
	raw, _ := json.Marshal(metadata)
	_, err := h.DB.ExecContext(ctx, `INSERT INTO worker_heartbeats(worker_id,kind,last_seen_at,metadata) VALUES($1,$2,$3,$4) ON CONFLICT(worker_id) DO UPDATE SET kind=EXCLUDED.kind,last_seen_at=EXCLUDED.last_seen_at,metadata=EXCLUDED.metadata`, h.WorkerID, h.Kind, time.Now().UTC(), raw)
	return err
}
