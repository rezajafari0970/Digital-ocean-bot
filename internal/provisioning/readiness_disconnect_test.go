package provisioning

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

type disconnectReadinessSSH struct {
	at, calls int
	err       error
}

func (s *disconnectReadinessSSH) RunDetailedObserved(ctx context.Context, t Target, k []byte, cmd string, o StageObserver) (CommandResult, error) {
	s.calls++
	if s.calls == s.at {
		return CommandResult{}, s.err
	}
	return (readinessSSHStub{}).RunDetailedObserved(ctx, t, k, cmd, o)
}

type disconnectReadinessRecorder struct {
	issues   []ReadinessIssue
	snapshot ReadinessSnapshot
}

func (r *disconnectReadinessRecorder) Readiness(_ context.Context, _ string, _ Target, s ReadinessSnapshot, i []ReadinessIssue) error {
	r.snapshot = s
	r.issues = i
	return nil
}
func TestReadinessDisconnectDuringReboot(t *testing.T) {
	for _, e := range []error{errors.New("connection reset by peer"), errors.New("broken pipe"), io.ErrUnexpectedEOF, io.EOF, fmt.Errorf("probe: %w", io.EOF)} {
		for _, at := range []int{1, 2, 6, 9, 14} {
			t.Run(e.Error()+"/probe-"+string(rune('A'+at)), func(t *testing.T) {
				ssh := &disconnectReadinessSSH{at: at, err: e}
				rec := &disconnectReadinessRecorder{}
				c := ReadinessCollector{SSH: ssh, Recorder: rec}
				s, err := c.Collect(context.Background(), "r", Target{}, nil, nil)
				var re *ReadinessError
				if !errors.As(err, &re) || !re.Retryable || s.Status != "BLOCKED" || len(re.Codes) != 1 || re.Codes[0] != "SSH_DISCONNECTED" {
					t.Fatalf("snapshot=%+v err=%v", s, err)
				}
				if s.RebootRequired {
					t.Fatal("missing reboot observation reported as reboot required")
				}
				if ssh.calls != at {
					t.Fatalf("continued probes/remediation after loss: %d > %d", ssh.calls, at)
				}
				if len(rec.issues) != 1 || rec.issues[0].Action != "RETRY" {
					t.Fatalf("wrong permanent issue %+v", rec.issues)
				}
				ssh.at = 0
				s, err = c.Collect(context.Background(), "r", Target{}, nil, nil)
				if err != nil || s.Status != "READY" {
					t.Fatalf("did not recover after boot: %+v %v", s, err)
				}
			})
		}
	}
}
func TestReadinessDisconnectDoesNotWeakenHostKeyChecks(t *testing.T) {
	for _, at := range []int{1, 6} {
		ssh := &disconnectReadinessSSH{at: at, err: ErrHostKeyMismatch}
		s, err := (ReadinessCollector{SSH: ssh}).Collect(context.Background(), "r", Target{}, nil, nil)
		var re *ReadinessError
		if !errors.As(err, &re) || re.Retryable || s.Status != "BLOCKED" || re.Codes[0] != "SSH_HOST_KEY_MISMATCH" {
			t.Fatalf("host key weakened: %+v %v", s, err)
		}
		if ssh.calls != at {
			t.Fatal("continued with invalid host key")
		}
	}
}

type disconnectPackageRecheckSSH struct {
	audits           int
	remediationError bool
	fault            error
}

func (s *disconnectPackageRecheckSSH) RunDetailedObserved(ctx context.Context, t Target, k []byte, cmd string, o StageObserver) (CommandResult, error) {
	if strings.Contains(cmd, "dpkg --audit") {
		s.audits++
		if s.audits == 1 {
			return CommandResult{Stderr: "packages unconfigured"}, ErrSSHCommand
		}
		if s.fault != nil {
			return CommandResult{}, s.fault
		}
		return CommandResult{}, errors.New("unexpected EOF")
	}
	if cmd == "dpkg --configure -a" && s.remediationError {
		if s.fault != nil {
			return CommandResult{}, s.fault
		}
		return CommandResult{Stderr: "broken pipe"}, ErrSSHCommand
	}
	return (readinessSSHStub{}).RunDetailedObserved(ctx, t, k, cmd, o)
}
func TestReadinessDisconnectDuringPackageRecovery(t *testing.T) {
	for _, fault := range []error{nil, io.EOF, io.ErrUnexpectedEOF, fmt.Errorf("repair: %w", io.EOF)} {
		for _, remediationError := range []bool{false, true} {
			ssh := &disconnectPackageRecheckSSH{remediationError: remediationError, fault: fault}
			s, err := (ReadinessCollector{SSH: ssh}).Collect(context.Background(), "r", Target{}, nil, nil)
			var re *ReadinessError
			if !errors.As(err, &re) || !re.Retryable || s.Status != "BLOCKED" || re.Codes[0] != "SSH_DISCONNECTED" {
				t.Fatalf("package loss misclassified: %+v %v", s, err)
			}
		}
	}
}
