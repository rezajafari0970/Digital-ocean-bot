package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"testing"
)

var errAuditPersistence = errors.New("step persistence unavailable")

type checkedFaultStore struct {
	memStore
	failFinish bool
	failUpdate State
	beginErr   error
}

func (s *checkedFaultStore) FinishStep(context.Context, string, string, error, ErrorClass) error {
	if s.failFinish {
		return errAuditPersistence
	}
	return nil
}
func (s *checkedFaultStore) Update(ctx context.Context, d *Deployment) error {
	if d.State == s.failUpdate {
		return errAuditPersistence
	}
	return s.memStore.Update(ctx, d)
}
func (s *checkedFaultStore) BeginStep(context.Context, string, string, int) (int, error) {
	return 1, s.beginErr
}

type missingInstallerSteps struct{ stepStub }

func (s *missingInstallerSteps) Provision(_ context.Context, d Deployment) (Deployment, error) {
	s.calls++
	return d, provisioning.ErrInstallerNotConfigured
}
func TestFinishStepFailureNeverAdvancesOrReportsReady(t *testing.T) {
	s := &checkedFaultStore{failFinish: true}
	steps := &stepStub{}
	d, err := (Engine{Store: s, Steps: steps}).Run(context.Background(), Request{AccountID: "a", ProfileID: "p"})
	if !errors.Is(err, errAuditPersistence) || d.State == Ready || steps.calls != 1 || s.d.CurrentStep != "create" {
		t.Fatal(d, err, steps.calls)
	}
}
func TestInstallerPlaceholderStateWriteFailureIsReported(t *testing.T) {
	s := &checkedFaultStore{failUpdate: WaitingInstaller}
	steps := &missingInstallerSteps{}
	d, err := (Engine{Store: s, Steps: steps}).Run(context.Background(), Request{AccountID: "a", ProfileID: "p"})
	if !errors.Is(err, errAuditPersistence) || s.d.State == WaitingInstaller || steps.calls != 3 {
		t.Fatal(d, err, steps.calls)
	}
}
func TestBeginStepDatabaseFailureDoesNotDeclareTerminal(t *testing.T) {
	s := &checkedFaultStore{beginErr: errAuditPersistence}
	steps := &stepStub{}
	d, err := (Engine{Store: s, Steps: steps}).Run(context.Background(), Request{AccountID: "a", ProfileID: "p"})
	if !errors.Is(err, errAuditPersistence) || d.State == Failed || s.d.State == Failed || steps.calls != 0 {
		t.Fatal(d, err, steps.calls)
	}
}

func (s *checkedFaultStore) Finalize(ctx context.Context, d *Deployment, step, message string, apply func(context.Context, DBTX) error) error {
	if d.State == s.failUpdate {
		return errAuditPersistence
	}
	return s.memStore.Finalize(ctx, d, step, message, apply)
}

func (s *checkedFaultStore) AdmitStep(ctx context.Context, d *Deployment, step string, max int) error {
	if s.beginErr != nil {
		return s.beginErr
	}
	if d.State == s.failUpdate {
		return errAuditPersistence
	}
	return s.memStore.AdmitStep(ctx, d, step, max)
}
