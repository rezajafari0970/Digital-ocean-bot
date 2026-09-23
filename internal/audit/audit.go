package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
	"time"
)

type Event struct {
	AccountID    string
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	Message      string
	Metadata     map[string]any
	At           time.Time
}
type Recorder interface {
	Record(context.Context, Event) error
}
type SQLRecorder struct{ DB *sql.DB }

func (r SQLRecorder) Record(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	e.Message = redact.Text(e.Message)
	raw, _ := json.Marshal(e.Metadata)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO audit_events(account_id,actor,action,resource_type,resource_id,result,message,metadata,created_at) VALUES(NULLIF($1,'')::uuid,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9)`, e.AccountID, e.Actor, e.Action, e.ResourceType, e.ResourceID, e.Result, e.Message, raw, e.At)
	return err
}
