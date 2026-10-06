package provisioning

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestInstallerPrerequisites(t *testing.T) {
	for _, tc := range []struct {
		name        string
		p           Plan
		readiness   bool
		want        []string
		placeholder string
	}{
		{"default", Plan{Scripts: DefaultPreInstallerSteps()}, true, []string{"ssh", "readiness", "bootstrap-memory-headroom", "bootstrap-cloud-init", "bootstrap-package-state", "bootstrap-package-index", "bootstrap-prerequisites", "bootstrap-kernel-memory-guard", "bootstrap-host-firewall-off", "bootstrap-verify"}, "panel"},
		{"legacy", Plan{Bootstrap: "prep", InstallPanel: "true", Verify: "verify"}, false, []string{"ssh", "bootstrap"}, "panel"},
		{"custom", Plan{Scripts: []ScriptStep{{Name: "custom", Category: "bootstrap", Execute: "prep"}, {Name: "selected", Category: "install", Execute: ":"}}}, true, []string{"ssh", "readiness", "custom"}, "selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, p, err := InstallerPrerequisites(tc.p, tc.readiness)
			if err != nil || p != tc.placeholder || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v %q %v", got, p, err)
			}
		})
	}
	if _, _, err := InstallerPrerequisites(Plan{Bootstrap: "prep", InstallPanel: "real", Verify: "verify"}, true); !errors.Is(err, ErrInvalidPlan) {
		t.Fatal("missing placeholder accepted")
	}
}

type checkpointStore struct {
	memStore
	completed map[string]bool
	beginErr  error
	finishErr error
}

func (s *checkpointStore) StepCompleted(_ context.Context, _ string, step string) (bool, error) {
	return s.completed[step], nil
}
func (s *checkpointStore) BeginStep(ctx context.Context, run, step string, max int) (int, error) {
	if s.beginErr != nil {
		return 0, s.beginErr
	}
	return s.memStore.BeginStep(ctx, run, step, max)
}
func (s *checkpointStore) FinishStep(_ context.Context, _ string, step string, err error, _ bool) error {
	if s.finishErr != nil {
		return s.finishErr
	}
	if err == nil {
		s.completed[step] = true
	}
	return nil
}
func TestInstallerBootstrapEngineResumeFaults(t *testing.T) {
	plan := Plan{Scripts: []ScriptStep{{Name: "gap", Category: "bootstrap", Execute: "prepare", Precheck: "check", MaxAttempts: 3}, {Name: "already", Category: "bootstrap", Execute: "nonrepeatable", MaxAttempts: 1}, {Name: "panel", Category: "install", Execute: "true"}}}
	for _, tc := range []struct {
		name          string
		begin, finish error
		wantRuns      int
		wantErr       error
	}{
		{"gap-with-completed-later", nil, nil, 1, ErrInstallerNotConfigured},
		{"backoff", ErrStepRetryDeferred, nil, 0, ErrStepRetryDeferred},
		{"terminal", ErrStepTerminal, nil, 0, ErrStepTerminal},
		{"budget", ErrStepRetryLimit, nil, 0, ErrStepRetryLimit},
		{"finish-write-fault", nil, errors.New("db-fault"), 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &checkpointStore{memStore: memStore{exists: true, r: Run{ID: "r", State: Pending, CurrentStep: "gap"}, attempts: map[string]int{"already": 1}}, completed: map[string]bool{"ssh": true, "already": true}, beginErr: tc.begin, finishErr: tc.finish}
			ssh := &sshStub{}
			got, err := (Engine{Store: store, SSH: ssh, Secrets: secretStub{}}).Execute(context.Background(), Target{}, plan)
			want := tc.wantErr
			if tc.finish != nil {
				want = tc.finish
			}
			if !errors.Is(err, want) || ssh.runs != tc.wantRuns || ssh.waits != 0 {
				t.Fatalf("run=%+v runs=%d err=%v", got, ssh.runs, err)
			}
			if store.attempts["already"] != 1 {
				t.Fatal("completed side effect repeated")
			}
			if tc.finish != nil && store.r.CurrentStep != "gap" {
				t.Fatal("advanced despite failed durability")
			}
			if tc.begin == nil && tc.finish == nil && got.State != WaitingInstaller {
				t.Fatal("did not reach real placeholder")
			}
		})
	}
}
