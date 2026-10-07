package clientops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// Only the runtime-acquisition boundary can prove that no client mutation ran.
// Never infer this property from a transport error returned after executeRuntime.
type runtimeUnavailable struct{ cause error }

func (e *runtimeUnavailable) Error() string {
	return "panel runtime unavailable before client mutation"
}
func (e *runtimeUnavailable) Unwrap() error { return e.cause }

// ExecutionFailure preserves exact safe identifiers if an internal/journal error
// requires the worker to close the global gate. Remote error text stays out of UI.
type ExecutionFailure struct {
	PanelID, JobID string
	Cause          error
}

func (e *ExecutionFailure) Error() string {
	return fmt.Sprintf("panel=%s job=%s: %v", e.PanelID, e.JobID, e.Cause)
}
func (e *ExecutionFailure) Unwrap() error { return e.Cause }

type panelFailure struct {
	code                       string
	beforeMutation, quarantine bool
}

func classifyPanelFailure(err error, attempts int) (panelFailure, bool) {
	var unavailable *runtimeUnavailable
	if errors.As(err, &unavailable) {
		return panelFailure{code: "RUNTIME_UNAVAILABLE", beforeMutation: true}, true
	}
	switch {
	case errors.Is(err, ErrClientConflict):
		return panelFailure{code: "CLIENT_IDENTITY_CONFLICT", quarantine: true}, true
	case errors.Is(err, ErrInboundMissing):
		return panelFailure{code: "INBOUND_MISSING", quarantine: attempts >= 3}, true
	case errors.Is(err, ErrVerify):
		return panelFailure{code: "VERIFICATION_FAILED", quarantine: attempts >= 3}, true
	case errors.Is(err, sanaei.ErrMutationRejected), errors.Is(err, sanaei.ErrInventoryRejected):
		return panelFailure{code: "PANEL_REJECTED", quarantine: attempts >= 3}, true
	case errors.Is(err, sanaei.ErrAPIRequest), errors.Is(err, sanaei.ErrSessionRequest), errors.Is(err, sanaei.ErrPanelResponse):
		return panelFailure{code: "PANEL_REQUEST_FAILED", quarantine: attempts >= 3}, true
	}
	// Unknown errors, SQL failures and internal invariants retain global closure.
	return panelFailure{}, false
}

// recordPanelFailure commits the job outcome and panel admission fence together.
// It never refunds finite authorization budgets or rearms a quarantined panel.
func (j Journal) recordPanelFailure(ctx context.Context, job Job, f panelFailure) error {
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempts int
	if err = tx.QueryRowContext(ctx, "SELECT attempts FROM client_mutation_jobs WHERE id=$1 AND panel_id=$2 AND state='RUNNING' FOR UPDATE", job.ID, job.PanelID).Scan(&attempts); err != nil {
		return err
	}
	if attempts != job.Attempts || attempts < 1 {
		return ErrInvalidRequest
	}
	state := "COOLDOWN"
	if f.quarantine {
		state = "QUARANTINED"
	}
	var actualState string
	var retry sql.NullTime
	err = tx.QueryRowContext(ctx, `INSERT INTO client_mutation_panel_health(panel_id,state,failures,retry_after,last_job_id,reason_code)
 VALUES($1,$2,1,CASE WHEN $2='COOLDOWN' THEN now()+interval '30 seconds' END,$3,$4)
 ON CONFLICT(panel_id) DO UPDATE SET
 state=CASE WHEN client_mutation_panel_health.state='QUARANTINED' THEN 'QUARANTINED' ELSE excluded.state END,
 failures=LEAST(client_mutation_panel_health.failures+1,1000000),
 retry_after=CASE WHEN client_mutation_panel_health.state='QUARANTINED' OR excluded.state='QUARANTINED' THEN NULL ELSE now()+LEAST(120,30*(client_mutation_panel_health.failures+1))*interval '1 second' END,
 last_job_id=CASE WHEN client_mutation_panel_health.state='QUARANTINED' THEN client_mutation_panel_health.last_job_id ELSE excluded.last_job_id END,
 reason_code=CASE WHEN client_mutation_panel_health.state='QUARANTINED' THEN client_mutation_panel_health.reason_code ELSE excluded.reason_code END,
 updated_at=now()
 RETURNING state,retry_after`, job.PanelID, state, job.ID, f.code).Scan(&actualState, &retry)
	if err != nil {
		return err
	}
	if actualState == "QUARANTINED" {
		_, err = tx.ExecContext(ctx, `UPDATE client_mutation_jobs SET state='FAILED',next_retry_at=NULL,completed_at=now(),last_error=$2,
 result=result||jsonb_build_object('isolation','QUARANTINED','reason_code',$2::text),updated_at=now() WHERE id=$1`, job.ID, f.code)
	} else {
		decrement := 0
		if f.beforeMutation {
			decrement = 1
		}
		_, err = tx.ExecContext(ctx, `UPDATE client_mutation_jobs SET state='PENDING',attempts=attempts-$4,next_retry_at=$2,last_error=$3,
 result=result||jsonb_build_object('isolation','COOLDOWN','reason_code',$3::text,'pre_mutation_deferrals',COALESCE((result->>'pre_mutation_deferrals')::int,0)+$4),
 updated_at=now() WHERE id=$1`, job.ID, retry.Time, f.code, decrement)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Useful for admission/status callers. No remote I/O and no implicit resume.
func (j Journal) PanelBlocked(ctx context.Context, panel string) (bool, error) {
	var blocked bool
	err := j.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM client_mutation_panel_health WHERE panel_id=$1 AND (state='QUARANTINED' OR retry_after>now()))`, panel).Scan(&blocked)
	return blocked, err
}

// Only approved codes cross into durable lifecycle diagnostics. Remote messages,
// URLs, returned emails and skipped-reason text can contain credentials.
func lifecycleErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if f, known := classifyPanelFailure(err, 1); known {
		return f.code
	}
	if errors.Is(err, ErrLifecycleSuperseded) {
		return "LIFECYCLE_SUPERSEDED"
	}
	if errors.Is(err, ErrLifecycleExpired) {
		return "LIFECYCLE_EXPIRED"
	}
	return "INTERNAL_EXECUTION_ERROR"
}
func sanitizeLifecycleReport(report map[string]any, responseCount, skippedCount int, postErr, verifyErr error) {
	report["response"] = map[string]int{"count": responseCount, "skipped_count": skippedCount}
	delete(report, "post_error")
	delete(report, "verify_error")
	if postErr != nil {
		report["post_error"] = lifecycleErrorCode(postErr)
	}
	if verifyErr != nil {
		report["verify_error"] = lifecycleErrorCode(verifyErr)
	}
}
