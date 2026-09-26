package testharness

import (
	"context"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func TestInstallerFailureMatrix(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		e, st, states, scripts, r, run := InstallerFixture(InstallerSuccess)
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
			t.Fatal(err)
		}
		if states.Last() != "INSTALL_COMPLETE" || scripts.Writes != 1 {
			t.Fatalf("state=%s writes=%d", states.Last(), scripts.Writes)
		}
		if st.Attempts["installer-g1-harness-v1-install"] != 1 {
			t.Fatal(st.Attempts)
		}
	})
	t.Run("retryable_then_success", func(t *testing.T) {
		e, st, states, scripts, r, run := InstallerFixture(InstallerRetryable)
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err == nil {
			t.Fatal("expected retryable failure")
		}
		if states.Last() != "INSTALLING" {
			t.Fatalf("state=%s", states.Last())
		}
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
			t.Fatal(err)
		}
		if states.Last() != "INSTALL_COMPLETE" || st.Attempts["installer-g1-harness-v1-install"] != 2 || scripts.Writes != 1 {
			t.Fatalf("state=%s attempts=%v writes=%d", states.Last(), st.Attempts, scripts.Writes)
		}
	})
	t.Run("terminal_requires_rollback", func(t *testing.T) {
		e, _, states, _, r, run := InstallerFixture(InstallerTerminal)
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err == nil {
			t.Fatal("expected terminal failure")
		}
		if states.Last() != "ROLLBACK_REQUIRED" {
			t.Fatalf("state=%s", states.Last())
		}
	})
	t.Run("verify_failure_requires_rollback", func(t *testing.T) {
		e, _, states, scripts, r, run := InstallerFixture(InstallerVerifyFailure)
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err == nil {
			t.Fatal("expected verify failure")
		}
		if states.Last() != "ROLLBACK_REQUIRED" || !scripts.Marker {
			t.Fatalf("state=%s marker=%v", states.Last(), scripts.Marker)
		}
	})
	t.Run("rollback_success", func(t *testing.T) {
		e, _, states, scripts, r, run := InstallerFixture(InstallerTerminal)
		_ = e.Execute(context.Background(), run, r, provisioning.Target{}, nil)
		scripts.Fault = InstallerSuccess
		scripts.Marker = true
		if err := e.Rollback(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
			t.Fatal(err)
		}
		if states.Last() != "ROLLED_BACK" || scripts.Marker {
			t.Fatalf("state=%s marker=%v", states.Last(), scripts.Marker)
		}
	})
	t.Run("rollback_failure", func(t *testing.T) {
		e, _, states, scripts, r, run := InstallerFixture(InstallerTerminal)
		_ = e.Execute(context.Background(), run, r, provisioning.Target{}, nil)
		scripts.Fault = InstallerRollbackFailure
		scripts.Marker = true
		if err := e.Rollback(context.Background(), run, r, provisioning.Target{}, nil); err == nil {
			t.Fatal("expected rollback failure")
		}
		if states.Last() != "FAILED" {
			t.Fatalf("state=%s", states.Last())
		}
	})
}

func TestInstallerCrashRecoveryMatrix(t *testing.T) {
	const install = "installer-g1-harness-v1-install"
	t.Run("crash_after_start_retries_latest_attempt", func(t *testing.T) {
		e, st, states, scripts, r, run := InstallerFixture(InstallerSuccess)
		if _, err := st.BeginStep(context.Background(), run.ProvisionRunID, install, 3); err != nil {
			t.Fatal(err)
		}
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
			t.Fatal(err)
		}
		if st.Attempts[install] != 2 || scripts.Writes != 1 || states.Last() != "INSTALL_COMPLETE" {
			t.Fatalf("attempts=%v writes=%d state=%s", st.Attempts, scripts.Writes, states.Last())
		}
	})
	t.Run("crash_after_side_effect_precheck_prevents_duplicate", func(t *testing.T) {
		e, st, states, scripts, r, run := InstallerFixture(InstallerSuccess)
		if _, err := st.BeginStep(context.Background(), run.ProvisionRunID, install, 3); err != nil {
			t.Fatal(err)
		}
		scripts.Marker = true
		scripts.Writes = 1
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
			t.Fatal(err)
		}
		if st.Attempts[install] != 2 || scripts.Writes != 1 || states.Last() != "INSTALL_COMPLETE" {
			t.Fatalf("attempts=%v writes=%d state=%s", st.Attempts, scripts.Writes, states.Last())
		}
	})
	t.Run("completed_latest_attempt_is_not_replayed", func(t *testing.T) {
		e, st, _, scripts, r, run := InstallerFixture(InstallerSuccess)
		if _, err := st.BeginStep(context.Background(), run.ProvisionRunID, install, 3); err != nil {
			t.Fatal(err)
		}
		scripts.Marker = true
		scripts.Writes = 1
		if err := st.FinishStep(context.Background(), run.ProvisionRunID, install, nil, false); err != nil {
			t.Fatal(err)
		}
		if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
			t.Fatal(err)
		}
		if st.Attempts[install] != 1 || scripts.Calls["install"] != 0 || scripts.Writes != 1 {
			t.Fatalf("attempts=%v calls=%v writes=%d", st.Attempts, scripts.Calls, scripts.Writes)
		}
	})
}

func TestInstallerGenerationsHaveIndependentAttempts(t *testing.T) {
	e, st, _, scripts, r, run := InstallerFixture(InstallerSuccess)
	if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
		t.Fatal(err)
	}
	run.ID = "installer-run-2"
	run.Generation = 2
	if err := e.Execute(context.Background(), run, r, provisioning.Target{}, nil); err != nil {
		t.Fatal(err)
	}
	if st.Attempts["installer-g1-harness-v1-install"] != 1 || st.Attempts["installer-g2-harness-v1-install"] != 1 {
		t.Fatalf("attempts=%v", st.Attempts)
	}
	if scripts.Writes != 1 {
		t.Fatalf("idempotent precheck should prevent duplicate side effect, writes=%d", scripts.Writes)
	}
}
