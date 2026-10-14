package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"testing"
)

type selectedInstallerSteps struct{ stepStub }

func (s *selectedInstallerSteps) Provision(_ context.Context, d Deployment) (Deployment, error) {
	s.calls++
	return d, provisioning.ErrInstallerHandoff
}
func TestSelectedInstallerWaitsWithoutConfigurationError(t *testing.T) {
	for _, fault := range []State{"", WaitingInstaller} {
		s := &checkedFaultStore{failUpdate: fault}
		steps := &selectedInstallerSteps{}
		d, err := (Engine{Store: s, Steps: steps}).Run(context.Background(), Request{AccountID: "a", ProfileID: "p"})
		if fault != "" {
			if !errors.Is(err, errAuditPersistence) {
				t.Fatal(err)
			}
			continue
		}
		if err != nil || d.State != WaitingInstaller || d.LastError != "" || d.CurrentStep != "provision" || s.d.State != WaitingInstaller || steps.calls != 3 {
			t.Fatal(d, err, steps.calls)
		}
	}
}
