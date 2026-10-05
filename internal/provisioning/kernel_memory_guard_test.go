package provisioning

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type kernelGuardFixture struct{ dir, root, drop, grub, cmdline, mem, boot, config, log, update string }

func newKernelGuardFixture(t *testing.T) kernelGuardFixture {
	t.Helper()
	d := t.TempDir()
	f := kernelGuardFixture{dir: d, root: filepath.Join(d, "state"), drop: filepath.Join(d, "grub.d", "99-dob-kho.cfg"), grub: filepath.Join(d, "grub.cfg"), cmdline: filepath.Join(d, "cmdline"), mem: filepath.Join(d, "meminfo"), boot: filepath.Join(d, "boot-id"), config: filepath.Join(d, "config-7.0.0-test"), log: filepath.Join(d, "calls"), update: filepath.Join(d, "bin", "update-grub")}
	for _, p := range []string{filepath.Dir(f.drop), filepath.Dir(f.update)} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.write(t, f.config, "CONFIG_KEXEC_HANDOVER=y\nCONFIG_KEXEC_HANDOVER_ENABLE_DEFAULT=y\n", 0600)
	f.write(t, filepath.Join(d, "osrelease"), "7.0.0-test\n", 0600)
	f.write(t, f.grub, "linux /boot/vmlinuz-test root=UUID=keep ro console=ttyS1\n", 0600)
	f.write(t, f.cmdline, "root=UUID=keep ro console=ttyS1\n", 0600)
	f.write(t, f.mem, "MemTotal: 1675908 kB\nCmaTotal: 0 kB\nCmaFree: 317272 kB\n", 0600)
	f.write(t, f.boot, "boot-before\n", 0600)
	f.generator(t, false, false)
	return f
}
func (f kernelGuardFixture) write(t *testing.T, p, s string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), mode); err != nil {
		t.Fatal(err)
	}
}
func (f kernelGuardFixture) generator(t *testing.T, fail, lost bool) {
	t.Helper()
	s := "#!/bin/sh\necho called >> '" + f.log + "'\n"
	if !fail {
		s += "printf '%s\\n' 'linux /boot/vmlinuz-test root=UUID=keep ro console=ttyS1 kho=off' > '" + f.grub + "'\n"
	}
	if fail || lost {
		s += "exit 1\n"
	}
	f.write(t, f.update, s, 0700)
}
func (f kernelGuardFixture) run(t *testing.T, mode string) (string, error) {
	t.Helper()
	replacer := strings.NewReplacer("/var/lib/digital-ocean-bot-kernel-memory", f.root, "/etc/default/grub.d/99-dob-kho.cfg", f.drop, "/boot/grub/grub.cfg", f.grub, "/proc/sys/kernel/random/boot_id", f.boot, "/proc/cmdline", f.cmdline, "/proc/meminfo", f.mem, "/proc/sys/kernel/osrelease", filepath.Join(f.dir, "osrelease"), "/boot/config-", filepath.Join(f.dir, "config-"), "OWNER_UID=0", "OWNER_UID="+strconv.Itoa(os.Getuid()))
	c := exec.Command("bash", "-c", replacer.Replace(kernelMemoryGuard(mode)))
	c.Env = append(os.Environ(), "PATH="+filepath.Join(f.dir, "bin")+":"+os.Getenv("PATH"))
	b, err := c.CombinedOutput()
	return string(b), err
}
func TestKernelMemoryGuardInterruptedRecoveryAndBootProof(t *testing.T) {
	for _, kind := range []string{"normal", "failed_generation", "lost_response"} {
		t.Run(kind, func(t *testing.T) {
			f := newKernelGuardFixture(t)
			if _, err := f.run(t, "check"); err == nil {
				t.Fatal("unprepared guard accepted")
			}
			if kind == "failed_generation" {
				f.generator(t, true, false)
				if out, err := f.run(t, "prepare"); err == nil {
					t.Fatal("failed generation accepted", out)
				}
				if _, err := f.run(t, "check"); err == nil {
					t.Fatal("partial state accepted")
				}
				f.generator(t, false, false)
			}
			if kind == "lost_response" {
				f.generator(t, false, true)
			}
			if out, err := f.run(t, "prepare"); err != nil {
				t.Fatal(out, err)
			}
			if out, err := f.run(t, "check"); err != nil {
				t.Fatal(out, err)
			}
			if out, err := f.run(t, "needs-reboot"); err != nil || strings.TrimSpace(out) != "yes" {
				t.Fatal("staged mitigation mistaken for active", out, err)
			}
			before, err := os.ReadFile(f.log)
			if err != nil {
				t.Fatal(err)
			}
			if out, err := f.run(t, "prepare"); err != nil {
				t.Fatal(out, err)
			}
			after, _ := os.ReadFile(f.log)
			if string(before) != string(after) {
				t.Fatal("idempotent run regenerated boot config")
			}
			original, _ := os.ReadFile(filepath.Join(f.root, "grub.before"))
			if !strings.Contains(string(original), "root=UUID=keep ro console=ttyS1\n") {
				t.Fatal("original boot arguments not preserved")
			}
			if out, err := f.run(t, "activation"); err == nil {
				t.Fatal("unchanged boot accepted", out)
			}
			f.write(t, f.boot, "boot-after\n", 0600)
			f.write(t, f.cmdline, "root=UUID=keep ro console=ttyS1 kho=off\n", 0600)
			f.write(t, f.mem, "MemTotal: 1675908 kB\nCmaTotal: 0 kB\nCmaFree: 0 kB\n", 0600)
			if out, err := f.run(t, "needs-reboot"); err != nil || strings.TrimSpace(out) != "no" {
				t.Fatal("boot activation not recognized", out, err)
			}
			if out, err := f.run(t, "activation"); err != nil {
				t.Fatal("activation proof failed", out, err)
			}
			out, err := f.run(t, "audit")
			if err != nil {
				t.Fatal(out, err)
			}
			var proof struct {
				Disabled bool   `json:"kho_disabled"`
				Boot     string `json:"boot_id"`
				Prepared bool   `json:"prepared"`
			}
			if json.Unmarshal([]byte(out), &proof) != nil || !proof.Disabled || !proof.Prepared || proof.Boot != "boot-after" {
				t.Fatal("incomplete activation proof", out)
			}
		})
	}
}
func TestKernelMemoryGuardUnaffectedAndOperatorState(t *testing.T) {
	for _, kind := range []string{"unaffected", "unmanaged_dropin", "symlink", "explicit_enable", "future_conflict", "corrupt_journal"} {
		t.Run(kind, func(t *testing.T) {
			f := newKernelGuardFixture(t)
			switch kind {
			case "unaffected":
				f.write(t, filepath.Join(f.dir, "osrelease"), "5.15.0-test\n", 0600)
				f.write(t, f.mem, "MemTotal: 2048000 kB\nCmaTotal: 0 kB\nCmaFree: 0 kB\n", 0600)
			case "unmanaged_dropin":
				f.write(t, f.drop, "# operator file\n", 0600)
			case "symlink":
				if err := os.Symlink(f.cmdline, f.drop); err != nil {
					t.Fatal(err)
				}
			case "explicit_enable":
				f.write(t, f.cmdline, "root=UUID=keep kho=on\n", 0600)
			case "future_conflict":
				f.write(t, filepath.Join(filepath.Dir(f.drop), "operator.cfg"), "GRUB_CMDLINE_LINUX=\"kho=on\"\n", 0600)
			case "corrupt_journal":
				if err := os.Mkdir(f.root, 0700); err != nil {
					t.Fatal(err)
				}
				f.write(t, filepath.Join(f.root, "state.json"), `{"version":7,"phase":"UNKNOWN"}`, 0600)
			}
			original, _ := os.ReadFile(f.grub)
			out, err := f.run(t, "prepare")
			if kind == "unaffected" {
				if err != nil {
					t.Fatal(out, err)
				}
			} else if err == nil {
				t.Fatal("unsafe state accepted", out)
			}
			after, _ := os.ReadFile(f.grub)
			if string(after) != string(original) {
				t.Fatal("unrelated boot config changed")
			}
			if _, err := os.Stat(f.log); !os.IsNotExist(err) {
				t.Fatal("boot generator unexpectedly invoked")
			}
		})
	}
}
func TestBootstrapKernelGuardBeforeInstallerAndReadiness(t *testing.T) {
	seenCloud, seenGuard := false, false
	for _, s := range DefaultPreInstallerSteps() {
		if s.Name == "bootstrap-cloud-init" {
			seenCloud = true
		}
		if s.Name == "bootstrap-kernel-memory-guard" {
			if !seenCloud || s.Execute != KernelMemoryGuardCommand() || s.Verify != KernelMemoryGuardReadyCommand() {
				t.Fatal("guard placement or verification differs")
			}
			seenGuard = true
		}
		if s.Name == "panel" && !seenGuard {
			t.Fatal("installer can run before kernel guard")
		}
	}
	if !seenGuard || !strings.Contains(KernelMemoryRebootRequiredCommand(), "needs-reboot") {
		t.Fatal("missing activation readiness")
	}
}
