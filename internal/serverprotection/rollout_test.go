package serverprotection

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGuardianUpgradeScope(t *testing.T) {
	for _, tc := range []struct {
		raw, panel string
		allowed    bool
	}{
		{"", "a", true}, {"none", "a", false}, {"a,b", "a", true},
		{"a,b", "c", false}, {" a, b ", "b", true},
	} {
		c := Controller{UpgradePanels: ParseUpgradePanels(tc.raw)}
		if got := c.upgradeAllowed(tc.panel); got != tc.allowed {
			t.Fatalf("scope %q panel %q: %v", tc.raw, tc.panel, got)
		}
	}
}

func TestGuardianServiceTransitionsDoNotRepeatReloads(t *testing.T) {
	for _, tc := range []struct {
		name, state, rc, fail string
		enabled, changed      bool
		want                  string
	}{
		{"healthy", "enabled", "0", "", true, false, "is-enabled\nstart\n"},
		{"stopped-but-enabled", "enabled", "0", "", true, false, "is-enabled\nstart\n"},
		{"first-enable", "disabled", "1", "", true, false, "is-enabled\nenable\nimplicit-reload\nstart\n"},
		{"runtime-is-not-persistent", "enabled-runtime", "0", "", true, false, "is-enabled\nenable\nimplicit-reload\nstart\n"},
		{"unknown-enable", "unexpected", "4", "", true, false, "is-enabled\nenable\nimplicit-reload\nstart\n"},
		{"unexpected-exit", "enabled", "5", "", true, false, "is-enabled\nenable\nimplicit-reload\nstart\n"},
		{"changed-binary", "enabled", "0", "", true, true, "is-enabled\nrestart\nstart\n"},
		{"disable", "enabled", "0", "", false, false, "is-enabled\ndisable\nimplicit-reload\nstop\n"},
		{"disabled-but-running", "disabled", "1", "", false, false, "is-enabled\nstop\n"},
		{"unknown-disable", "unknown", "4", "", false, false, "is-enabled\ndisable\nimplicit-reload\nstop\n"},
		{"enable-fails", "disabled", "1", "enable", true, false, "is-enabled\nenable\n"},
		{"start-fails", "enabled", "0", "start", true, false, "is-enabled\nstart\n"},
		{"disable-fails", "enabled", "0", "disable", false, false, "is-enabled\ndisable\n"},
		{"stop-fails", "disabled", "1", "stop", false, false, "is-enabled\nstop\n"},
		{"restart-fails", "enabled", "0", "restart", true, true, "is-enabled\nrestart\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := guardianServiceFixture(t, tc.state, tc.rc, tc.fail)
			changed := "0"
			if tc.changed {
				changed = "1"
			}
			command := "unit_changed=" + changed + "\n" + guardianServiceCommand(tc.enabled) + "printf receipt > \"$FIXTURE/receipt\"\n"
			cmd := exec.Command("bash", "-eu", "-c", command)
			cmd.Env = append(os.Environ(), "FIXTURE="+root, "PATH="+root+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err != nil) != (tc.fail != "") {
				t.Fatal(err, string(out))
			}
			calls, _ := os.ReadFile(filepath.Join(root, "calls"))
			if string(calls) != tc.want {
				t.Fatalf("calls %q want %q", calls, tc.want)
			}
			_, err = os.Stat(filepath.Join(root, "receipt"))
			if (err == nil) != (tc.fail == "") {
				t.Fatal("failure emitted receipt", err)
			}
		})
	}
}
func guardianServiceFixture(t *testing.T, state, rc, fail string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{"state": state, "rc": rc, "fail": fail} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fake := `#!/bin/bash
set -eu
printf '%s\n' "$1" >> "$FIXTURE/calls"
if [ "$(cat "$FIXTURE/fail")" = "$1" ]; then exit 7; fi
case "$1" in
 is-enabled) cat "$FIXTURE/state";exit "$(cat "$FIXTURE/rc")" ;;
 enable) printf enabled > "$FIXTURE/state";printf 0 > "$FIXTURE/rc";printf 'implicit-reload\n' >> "$FIXTURE/calls" ;;
 disable) printf disabled > "$FIXTURE/state";printf 1 > "$FIXTURE/rc";printf 'implicit-reload\n' >> "$FIXTURE/calls" ;;
 start|restart) printf active > "$FIXTURE/active" ;;
 stop) printf inactive > "$FIXTURE/active" ;;
 daemon-reload) : ;;
 *) exit 66 ;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "systemctl"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestGuardianOwnedUnitChangeAndSteadyStateShell(t *testing.T) {
	root := guardianServiceFixture(t, "disabled", "1", "")
	command := "unit_changed=0\nunit=\"$FIXTURE/unit\"\n" + guardianUnitCommand() + guardianServiceCommand(true)
	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", "-eu", "-c", command)
		cmd.Env = append(os.Environ(), "FIXTURE="+root, "PATH="+root+":"+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	got := string(calls)
	if got != "daemon-reload\nis-enabled\nrestart\nenable\nimplicit-reload\nstart\nis-enabled\nstart\n" {
		t.Fatal(got)
	}
	unit, err := os.ReadFile(filepath.Join(root, "unit"))
	if err != nil || string(unit) != Unit {
		t.Fatal("unit content changed", err)
	}
}
