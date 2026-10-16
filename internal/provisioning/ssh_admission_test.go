package provisioning

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Use the actual installed OpenSSH parser against an isolated configuration.
// Only systemd scheduling/reload is substituted; no production sshd is changed.
func TestSSHAdmissionReconnectRollbackAndPreservation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root ownership fixture required")
	}
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		t.Skip("OpenSSH parser unavailable")
	}
	for _, tc := range []string{"verified", "missing_timer", "timer_preserves_operator", "timer_rollback", "interrupted_apply", "operator_policy", "unmanaged_file", "symlink", "reload_failure"} {
		t.Run(tc, func(t *testing.T) {
			dir := t.TempDir()
			main := filepath.Join(dir, "sshd_config")
			includes := filepath.Join(dir, "conf.d")
			drop := filepath.Join(includes, "00-dob-admission.conf")
			root := filepath.Join(dir, "managed")
			calls := filepath.Join(dir, "calls")
			timer := filepath.Join(dir, "timer.json")
			fail := filepath.Join(dir, "fail-reload")
			mustWrite := func(p, s string, mode os.FileMode) {
				t.Helper()
				if e := os.WriteFile(p, []byte(s), mode); e != nil {
					t.Fatal(e)
				}
			}
			if e := os.Mkdir(includes, 0755); e != nil {
				t.Fatal(e)
			}
			key := filepath.Join(dir, "host_key")
			if out, e := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); e != nil {
				t.Fatalf("key fixture: %v %s", e, out)
			}
			config := "Include " + includes + "/*.conf\nHostKey " + key + "\nPort 22222\nPasswordAuthentication no\nPermitRootLogin prohibit-password\n"
			if tc == "operator_policy" {
				config += "LoginGraceTime 45\n"
			}
			mustWrite(main, config, 0600)
			wrapper := filepath.Join(dir, "sshd")
			mustWrite(wrapper, "#!/bin/sh\nexec '"+sshd+"' -f '"+main+"' \"$@\"\n", 0700)
			ctl := filepath.Join(dir, "systemctl")
			mock := "#!/usr/bin/python3\nimport sys,pathlib,json\na=sys.argv[1:]\nwith pathlib.Path(" + quotePython(calls) + ").open('a') as f:f.write(json.dumps(a)+'\\n')\nif a[0]=='reload' and pathlib.Path(" + quotePython(fail) + ").exists():raise SystemExit(1)\n"
			if tc == "missing_timer" {
				mock += "if a[0]=='stop' and not pathlib.Path(" + quotePython(timer) + ").exists():\n sys.stderr.write('Unit not loaded.\\n');raise SystemExit(5)\n"
			}
			mustWrite(ctl, mock, 0700)
			runner := filepath.Join(dir, "systemd-run")
			mustWrite(runner, "#!/usr/bin/python3\nimport pathlib,json,sys\npathlib.Path("+quotePython(timer)+").write_text(json.dumps(sys.argv[1:]))\n", 0700)
			convert := func(s string) string {
				return strings.NewReplacer("/etc/ssh/sshd_config.d/00-dob-admission.conf", drop, "/etc/ssh/sshd_config", main, "/var/lib/digital-ocean-bot-ssh", root, "/usr/sbin/sshd", wrapper, "/usr/bin/systemctl", ctl, "/usr/bin/systemd-run", runner).Replace(s)
			}
			run := func(command string) error {
				t.Helper()
				out, e := exec.Command("bash", "-c", convert(command)).CombinedOutput()
				if e != nil {
					t.Logf("expected/fault result: %v %s", e, out)
				}
				return e
			}
			state := func() string {
				t.Helper()
				raw, e := os.ReadFile(filepath.Join(root, "state.json"))
				if e != nil {
					t.Fatal(e)
				}
				var v map[string]any
				if e = json.Unmarshal(raw, &v); e != nil {
					t.Fatal(e)
				}
				return v["state"].(string)
			}
			rollback := func() {
				t.Helper()
				raw, e := os.ReadFile(timer)
				if e != nil {
					t.Fatal(e)
				}
				var args []string
				if e = json.Unmarshal(raw, &args); e != nil {
					t.Fatal(e)
				}
				script := args[len(args)-1]
				if out, e := exec.Command("python3", "-c", script).CombinedOutput(); e != nil {
					t.Fatalf("rollback failed: %v %s", e, out)
				}
			}
			if tc == "operator_policy" {
				if e := run(SSHAdmissionReadyCommand()); e != nil {
					t.Fatal(e)
				}
				if e := run(SSHAdmissionCommand()); e != nil {
					t.Fatal(e)
				}
				if _, e := os.Stat(root); !os.IsNotExist(e) {
					t.Fatal("operator policy mutated")
				}
				return
			}
			if tc == "unmanaged_file" {
				mustWrite(drop, "# unrelated owner\n", 0600)
			}
			if tc == "symlink" {
				if e := os.Symlink(main, drop); e != nil {
					t.Fatal(e)
				}
			}
			if tc == "unmanaged_file" || tc == "symlink" {
				if run(SSHAdmissionCommand()) == nil {
					t.Fatal("unmanaged configuration accepted")
				}
				got, _ := os.ReadFile(main)
				if string(got) != config {
					t.Fatal("main config changed")
				}
				return
			}
			if run(SSHAdmissionReadyCommand()) == nil {
				t.Fatal("default host reported protected")
			}
			if tc == "reload_failure" {
				mustWrite(fail, "1", 0600)
			}
			e := run(SSHAdmissionCommand())
			if tc == "reload_failure" {
				if e == nil {
					t.Fatal("reload failure hidden")
				}
				if _, e := os.Stat(drop); !os.IsNotExist(e) {
					t.Fatal("failed reload retained candidate")
				}
				got, _ := os.ReadFile(main)
				if string(got) != config {
					t.Fatal("main changed")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if state() != "APPLIED" {
				t.Fatal("apply committed without fresh connection")
			}
			got, _ := os.ReadFile(main)
			if string(got) != config {
				t.Fatal("original SSH config overwritten")
			}
			effective, e := exec.Command(wrapper, "-T").CombinedOutput()
			if e != nil {
				t.Fatal(e, string(effective))
			}
			for _, v := range []string{"port 22222\n", "passwordauthentication no\n", "logingracetime 30\n", "maxstartups 20:30:60\n", "persourcemaxstartups 8\n"} {
				if !strings.Contains(string(effective), v) {
					t.Fatal("effective policy mismatch", v)
				}
			}
			if tc == "timer_preserves_operator" {
				mustWrite(drop, "# operator change\nLoginGraceTime 45\n", 0600)
				rollback()
				raw, _ := os.ReadFile(drop)
				if !strings.Contains(string(raw), "operator change") {
					t.Fatal("rollback removed operator change")
				}
				return
			}
			if tc == "timer_rollback" {
				rollback()
				if _, e := os.Stat(drop); !os.IsNotExist(e) {
					t.Fatal("unverified change not rolled back")
				}
				if state() != "ROLLED_BACK" {
					t.Fatal("rollback not journaled")
				}
				if run(SSHAdmissionReadyCommand()) == nil {
					t.Fatal("rollback counted as applied")
				}
				return
			}
			if tc == "interrupted_apply" {
				p := filepath.Join(root, "state.json")
				raw, _ := os.ReadFile(p)
				mustWrite(p, strings.Replace(string(raw), "APPLIED", "PREPARING", 1), 0600)
				if e = run(SSHAdmissionCommand()); e != nil {
					t.Fatal(e)
				}
				if state() != "APPLIED" {
					t.Fatal("interrupted reload not recovered")
				}
			}
			if e = run(SSHAdmissionReadyCommand()); e != nil {
				t.Fatal(e)
			}
			if state() != "COMMITTED" {
				t.Fatal("fresh verification not committed")
			}
			rollback()
			if _, e = os.Stat(drop); e != nil {
				t.Fatal("committed change rolled back", e)
			}
			if e = run(SSHAdmissionCommand()); e != nil {
				t.Fatal("idempotence", e)
			}
		})
	}
}
func quotePython(s string) string { b, _ := json.Marshal(s); return string(b) }
