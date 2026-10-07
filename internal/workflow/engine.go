package workflow

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"time"
)

var ErrStepUnavailable = errors.New("workflow step unavailable")

type Steps interface {
	Create(context.Context, Deployment) (Deployment, error)
	WaitResource(context.Context, Deployment) (Deployment, error)
	Provision(context.Context, Deployment) (Deployment, error)
	ImportDatabase(context.Context, Deployment) (Deployment, error)
	ConfigurePanel(context.Context, Deployment) (Deployment, error)
	RegisterClients(context.Context, Deployment, Request) (Deployment, error)
	RegisterTraffic(context.Context, Deployment) (Deployment, error)
}
type ReadyFinalizer interface {
	MarkReady(context.Context, Deployment) error
}
type FailureFinalizer interface {
	MarkFailed(context.Context, Deployment) error
}

type Engine struct {
	Store            Store
	Steps            Steps
	Finalizer        ReadyFinalizer
	FailureFinalizer FailureFinalizer
	RunLock          interface {
		Lock()
		Unlock()
	}
	RunLease        RunLease
	PostInstallOnly bool
}

func (e Engine) Run(ctx context.Context, req Request) (Deployment, error) {
	if e.RunLock != nil {
		e.RunLock.Lock()
		defer e.RunLock.Unlock()
	}
	d, _, err := e.Store.Reserve(ctx, req)
	if err != nil {
		return d, err
	}
	if e.RunLease != nil {
		release, lockErr := e.RunLease.Acquire(ctx, d.ID)
		if lockErr != nil {
			return d, lockErr
		}
		defer release()
		// Another runner may have completed or advanced the deployment before
		// this lease was acquired. Re-read persisted state under the lease.
		d, _, err = e.Store.Reserve(ctx, Request{DeploymentID: d.ID, AccountID: d.AccountID, ProfileID: d.ProfileID})
		if err != nil {
			return d, err
		}
	}
	if terminalDeploymentState(d.State) {
		return d, nil
	}
	postInstallContinuation := e.PostInstallOnly
	if d.State == DatabaseComplete {
		d.State = ConfiguringPanel
		d.CurrentStep = "panel"
		d.LastError = ""
		if err := e.transition(ctx, &d, "panel", "post-database continuation"); err != nil {
			return d, err
		}
	}
	if d.State == InstallComplete {
		d.State = ImportingDatabase
		d.CurrentStep = "database"
		d.LastError = ""
		if err := e.transition(ctx, &d, "database", "post-installer continuation"); err != nil {
			return d, err
		}
	}
	steps := []struct {
		name  string
		state State
		fn    func(context.Context, Deployment) (Deployment, error)
	}{{"create", Creating, e.Steps.Create}, {"wait_resource", WaitingResource, e.Steps.WaitResource}, {"provision", Provisioning, e.Steps.Provision}, {"database", ImportingDatabase, e.Steps.ImportDatabase}, {"panel", ConfiguringPanel, e.Steps.ConfigurePanel}, {"clients", RegisteringClients, func(c context.Context, x Deployment) (Deployment, error) { return e.Steps.RegisterClients(c, x, req) }}, {"traffic", RegisteringTraffic, e.Steps.RegisterTraffic}}
	for _, step := range steps {
		if done(d.CurrentStep, step.name) {
			continue
		}
		var recovered bool
		if durable, ok := e.Store.(completedStepStore); ok {
			d, recovered, err = durable.ReadCompletedStep(ctx, d, step.name)
			if err != nil {
				return d, err
			}
		}
		if !recovered {
			policy := DefaultStepPolicies[step.name]
			d.State = step.state
			d.CurrentStep = step.name
			d.Attempt++
			beginErr := e.Store.AdmitStep(ctx, &d, step.name, policy.MaxAttempts)
			if beginErr != nil {
				if !errors.Is(beginErr, ErrStepRetryLimit) && !errors.Is(beginErr, ErrStepTerminal) {
					return d, beginErr
				}
				d.State = Failed
				d.CurrentStep = "done"
				d.LastError = beginErr.Error()
				code := "STEP_TERMINAL_FAILURE"
				if errors.Is(beginErr, ErrStepRetryLimit) {
					code = "STEP_RETRY_LIMIT"
				}
				if ferr := e.persistTerminal(ctx, &d, step.name, code); ferr != nil {
					return d, errors.Join(beginErr, ferr)
				}
				return d, beginErr
			}
			budget := policy.Timeout + 10*time.Second
			if policy.Timeout <= 0 {
				budget = 3 * time.Minute
			}
			stageCtx, finish, stageErr := supervision.Begin(ctx, step.name, budget)
			if stageErr != nil {
				return d, stageErr
			}
			stepCtx := stageCtx
			cancel := func() {}
			if policy.Timeout > 0 {
				stepCtx, cancel = context.WithTimeout(stageCtx, policy.Timeout)
			}
			d, err = step.fn(stepCtx, d)
			cancel()
			finish()
			if errors.Is(err, provisioning.ErrInstallerNotConfigured) {
				d.State = WaitingInstaller
				d.CurrentStep = "provision"
				d.LastError = "INSTALLER_NOT_CONFIGURED"
				if ferr := e.finishStep(ctx, d, step.name, nil, ErrorClass("")); ferr != nil {
					return d, fmt.Errorf("persist installer placeholder: %w", ferr)
				}
				d.State = WaitingInstaller
				d.CurrentStep = "provision"
				d.LastError = "INSTALLER_NOT_CONFIGURED"
				if uerr := e.transition(ctx, &d, "provision", "installer not configured"); uerr != nil {
					return d, uerr
				}
				return d, nil
			}
			class := ClassifyStepError(step.name, err)
			if ferr := e.finishStep(ctx, d, step.name, err, class); ferr != nil {
				return d, errors.Join(err, fmt.Errorf("persist workflow step %s: %w", step.name, ferr))
			}
			if err != nil {
				d.LastError = err.Error()
				if !RetryableClass(class) {
					d.State = Failed
					d.CurrentStep = "done"
				}
				if d.State == Failed {
					if ferr := e.persistTerminal(ctx, &d, step.name, "step failed class="+string(class)); ferr != nil {
						return d, errors.Join(err, ferr)
					}
				} else {
					if uerr := e.transition(ctx, &d, step.name, "step failed class="+string(class)); uerr != nil {
						return d, errors.Join(err, uerr)
					}
				}
				return d, err
			}
		}
		if recovered && d.State == WaitingInstaller {
			if err := e.transition(ctx, &d, d.CurrentStep, "durable step continuation"); err != nil {
				return d, err
			}
			return d, nil
		}
		if postInstallContinuation && step.name == "database" {
			d.State = DatabaseComplete
			d.CurrentStep = "database_complete"
			d.LastError = ""
			if err := e.transition(ctx, &d, "database", "post-install database complete"); err != nil {
				return d, err
			}
			// Continue immediately into panel configuration; recovery can also resume from DATABASE_COMPLETE.
			continue
		}
		if postInstallContinuation && step.name == "panel" {
			d.State = PanelComplete
			d.CurrentStep = "panel_complete"
			d.LastError = ""
			if err := e.persistTerminal(ctx, &d, "panel", "post-install panel complete"); err != nil {
				return d, err
			}
			return d, nil
		}
		d.CurrentStep = next(step.name)
		if err := e.transition(ctx, &d, d.CurrentStep, "durable step continuation"); err != nil {
			return d, err
		}
	}

	d.State = Ready
	d.CurrentStep = "done"
	d.LastError = ""
	if err := e.persistTerminal(ctx, &d, "done", ""); err != nil {
		return d, err
	}
	return d, nil
}

func done(current, target string) bool { return rank(current) > rank(target) }
func rank(s string) int {
	m := map[string]int{"create": 0, "wait_resource": 1, "provision": 2, "database": 3, "panel": 4, "clients": 5, "traffic": 6, "done": 7}
	return m[s]
}
func next(s string) string {
	m := map[string]string{"create": "wait_resource", "wait_resource": "provision", "provision": "database", "database": "panel", "panel": "clients", "clients": "traffic", "traffic": "done"}
	return m[s]
}

func terminalDeploymentState(s State) bool {
	switch s {
	case Ready, Failed, InstallFailed, InstallRolledBack, PanelComplete:
		return true
	default:
		return false
	}
}

type completedStepStore interface {
	ReadCompletedStep(context.Context, Deployment, string) (Deployment, bool, error)
	FinishStepResult(context.Context, Deployment, string, error, ErrorClass) error
}

func (e Engine) finishStep(ctx context.Context, d Deployment, step string, err error, class ErrorClass) error {
	if durable, ok := e.Store.(completedStepStore); ok {
		return durable.FinishStepResult(ctx, d, step, err, class)
	}
	return e.Store.FinishStep(ctx, d.ID, step, err, class)
}

func (e Engine) transition(ctx context.Context, d *Deployment, step, message string) error {
	return e.Store.Finalize(ctx, d, step, message, nil)
}
func (e Engine) persistTerminal(ctx context.Context, d *Deployment, step, message string) error {
	return e.Store.Finalize(ctx, d, step, message, func(ctx context.Context, tx DBTX) error {
		if d.State == Failed && e.FailureFinalizer != nil {
			f, ok := e.FailureFinalizer.(interface {
				MarkFailedIn(context.Context, DBTX, Deployment) error
			})
			if !ok {
				return fmt.Errorf("failure finalizer lacks transactional capability")
			}
			return f.MarkFailedIn(ctx, tx, *d)
		}
		if d.State != Failed && e.Finalizer != nil {
			f, ok := e.Finalizer.(interface {
				MarkReadyIn(context.Context, DBTX, Deployment) error
			})
			if !ok {
				return fmt.Errorf("ready finalizer lacks transactional capability")
			}
			return f.MarkReadyIn(ctx, tx, *d)
		}
		return nil
	})
}
