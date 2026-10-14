package provisioning

import (
	"context"
	"errors"
	"testing"
)

type observedPrecheckStub struct {
	failure error
	stderr  string
	calls   int
	exit    *int
	signal  string
}

func (s *observedPrecheckStub) RunDetailedObserved(_ context.Context, _ Target, _ []byte, cmd string, observe StageObserver) (CommandResult, error) {
	s.calls++
	err := error(nil)
	code := 0
	if cmd == "check" {
		err = s.failure
		if err != nil {
			code = 1
		}
	}
	if observe != nil {
		observe(StageObservation{Stage: StageTCP})
		observe(StageObservation{Stage: StageCommand, Err: err})
	}
	exit := &code
	if cmd == "check" && err != nil {
		exit = s.exit
	}
	return CommandResult{ExitCode: exit, Signal: s.signal, Stderr: s.stderr}, err
}
func TestPrecheckMissIsNotDuplicateStageFailure(t *testing.T) {
	one, two, missingCmd := 1, 2, 127
	for _, tc := range []struct {
		name, stderr string
		err          error
		failure      bool
		calls        int
		exit         *int
		signal       string
		stageFailure bool
	}{
		{"already done", "", nil, false, 2, nil, "", false},
		{"predicate miss", "not installed", ErrSSHCommand, false, 3, &one, "", false},
		{"dns failure", "Temporary failure resolving archive.ubuntu.com", ErrSSHCommand, true, 1, &one, "", true},
		{"transport failure", "", errors.New("connection lost"), true, 1, nil, "", true},
		{"command missing", "command not found", ErrSSHCommand, true, 1, &missingCmd, "", true},
		{"syntax error", "syntax error", ErrSSHCommand, true, 1, &two, "", true},
		{"no exit evidence", "unknown failure", ErrSSHCommand, true, 1, nil, "", true},
		{"signalled", "killed", ErrSSHCommand, true, 1, &one, "KILL", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &observedPrecheckStub{failure: tc.err, stderr: tc.stderr, exit: tc.exit, signal: tc.signal}
			failures, tcp, phases := 0, 0, 0
			err := (SSHScriptRunner{SSH: stub}).RunScript(context.Background(), Target{}, nil, ScriptStep{Precheck: "check", Execute: "install", Verify: "verify"}, func(p ScriptPhaseResult) {
				if p.Phase == "precheck" && p.Err != nil {
					phases++
				}
			}, func(o StageObservation) {
				if o.Stage == StageTCP {
					tcp++
				}
				if o.Err != nil {
					failures++
				}
			})
			if (err != nil) != tc.failure || stub.calls != tc.calls || tcp != tc.calls {
				t.Fatal(err, stub.calls, tcp)
			}
			if tc.stageFailure && failures != 1 || !tc.stageFailure && failures != 0 {
				t.Fatal("wrong stage failure", failures)
			}
			if tc.err != nil && phases != 1 {
				t.Fatal("lost phase evidence")
			}
		})
	}
}
