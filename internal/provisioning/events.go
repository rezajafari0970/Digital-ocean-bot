package provisioning

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
)

type EventRecorder interface {
	Event(context.Context, Event) error
}

type Event struct {
	RunID       string
	Step        string
	Substep     string
	State       string
	Attempt     int
	Diagnostic  Diagnostic
	Duration    time.Duration
	Retryable   bool
	NextRetryAt *time.Time
	Metadata    map[string]any
}

func (s SQLStore) Event(ctx context.Context, e Event) error {
	meta, _ := json.Marshal(e.Metadata)
	var exit any
	if e.Diagnostic.ExitCode != nil {
		exit = *e.Diagnostic.ExitCode
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO provision_events(id,run_id,step,substep,state,attempt,error_code,error_class,error_message,error_fingerprint,exit_code,signal,stdout_tail,stderr_tail,duration_ms,retryable,next_retry_at,metadata)
VALUES(gen_random_uuid(),$1,$2,NULLIF($3,''),$4,NULLIF($5,0),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),$14,$15,$16,$17)`,
		e.RunID, e.Step, e.Substep, e.State, e.Attempt, e.Diagnostic.Code, string(e.Diagnostic.Class), redact.Text(e.Diagnostic.Message), e.Diagnostic.Fingerprint, exit, e.Diagnostic.Signal, tailDiagnostic(e.Diagnostic.StdoutTail), tailDiagnostic(e.Diagnostic.StderrTail), e.Duration.Milliseconds(), e.Retryable, e.NextRetryAt, meta)
	return err
}
