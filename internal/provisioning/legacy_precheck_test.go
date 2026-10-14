package provisioning

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These strings are the prechecks captured from the immutable live catalog.
var recordedLegacyPrechecks = []string{
	`set -euo pipefail; systemctl is-active x-ui >/dev/null; command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui`,
	`set -euo pipefail; systemctl is-active x-ui >/dev/null; (command -v x-ui >/dev/null || test -x /usr/local/x-ui/x-ui); test -f /etc/x-ui/x-ui.db`,
}

type legacyPredicateRunner struct {
	command, bin              string
	status                    int
	executions, verifications int
}

func (s *legacyPredicateRunner) RunDetailedObserved(ctx context.Context, _ Target, _ []byte, cmd string, observe StageObserver) (CommandResult, error) {
	if cmd == "record-install" {
		s.executions++
		zero := 0
		return CommandResult{ExitCode: &zero}, nil
	}
	if cmd == "verify-install" {
		s.verifications++
		zero := 0
		return CommandResult{ExitCode: &zero}, nil
	}
	if cmd != s.command {
		return CommandResult{}, errors.New("unexpected fixture command")
	}
	child := exec.CommandContext(ctx, "bash", "-c", cmd)
	child.Env = append(os.Environ(), "PATH="+s.bin, "SYSTEMCTL_FIXTURE_STATUS="+fmt.Sprint(s.status))
	out, err := child.CombinedOutput()
	status := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return CommandResult{}, err
		}
		status = exit.ExitCode()
		err = fmt.Errorf("%w: exit %d", ErrSSHCommand, status)
	}
	if observe != nil {
		observe(StageObservation{Stage: StageCommand, Err: err})
	}
	return CommandResult{ExitCode: &status, Stderr: string(out)}, err
}
func TestRecordedSanaeiInactiveAndAbsentPredicatesAllowInstall(t *testing.T) {
	for i, command := range recordedLegacyPrechecks {
		for _, code := range []int{3, 4} {
			for _, installer := range []bool{false, true} {
				t.Run(fmt.Sprintf("version%d-exit%d-installer%v", i+1, code, installer), func(t *testing.T) {
					dir := t.TempDir()
					if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit \"$SYSTEMCTL_FIXTURE_STATUS\"\n"), 0700); err != nil {
						t.Fatal(err)
					}
					runner := &legacyPredicateRunner{command: command, bin: dir, status: code}
					events := &eventRecorderStub{}
					step := ScriptStep{Name: "legacy-sanaei", Category: "install", Precheck: command, Execute: "record-install", Verify: "verify-install", MaxAttempts: 1}
					scripts := SSHScriptRunner{SSH: runner}
					var err error
					if installer {
						err = (InstallerExecutor{Store: &memStore{}, States: &installerStateStub{}, Scripts: scripts, Events: events}).Execute(context.Background(), InstallerRun{}, ResolvedInstaller{Steps: []ScriptStep{step}}, Target{}, nil)
					} else {
						_, err = (Engine{Store: &memStore{}, Secrets: secretStub{}, SSH: &sshStub{}, Scripts: scripts, Events: events}).Execute(context.Background(), Target{}, Plan{Scripts: []ScriptStep{step}})
					}
					if err != nil || runner.executions != 1 || runner.verifications != 1 {
						t.Fatal(err, runner)
					}
					found := false
					for _, event := range events.events {
						if event.Diagnostic.Code != "" {
							t.Fatal("false diagnostic", event)
						}
						if event.State == "PRECHECK_MISS" {
							found = true
						}
					}
					if !found {
						t.Fatal("missing expected predicate event", events.events)
					}
				})
			}
		}
	}
}
func TestLegacyPredicateCompatibilityDoesNotAuthorizeOtherFailures(t *testing.T) {
	for _, command := range recordedLegacyPrechecks {
		for _, code := range []int{1, 2, 3, 4, 126, 127, 137} {
			result := CommandResult{ExitCode: &code}
			want := code == 1 || code == 3 || code == 4
			if got := expectedPrecheckMiss(ErrSSHCommand, result, command); got != want {
				t.Fatal(code, got)
			}
			result.Stderr = "Failed to connect to bus: transport unavailable"
			if expectedPrecheckMiss(ErrSSHCommand, result, command) {
				t.Fatal("stderr fault ignored", code)
			}
			for _, whitespace := range []string{" ", "\n", "\t", "\r\n"} {
				result.Stderr = whitespace
				if expectedPrecheckMiss(ErrSSHCommand, result, command) {
					t.Fatal("whitespace stderr accepted", code)
				}
			}
			result.Stderr = ""
			result.Signal = "TERM"
			if expectedPrecheckMiss(ErrSSHCommand, result, command) {
				t.Fatal("signal ignored", code)
			}
			result.Signal = ""
			if code == 3 || code == 4 {
				for _, other := range []string{"systemctl is-active x-ui", command + "; exit 4", "false; " + command, "test -f arbitrary"} {
					if expectedPrecheckMiss(ErrSSHCommand, result, other) {
						t.Fatal("broadened compatibility", other)
					}
				}
			}
		}
		if expectedPrecheckMiss(ErrSSHCommand, CommandResult{}, command) {
			t.Fatal("missing exit accepted")
		}
	}
}
