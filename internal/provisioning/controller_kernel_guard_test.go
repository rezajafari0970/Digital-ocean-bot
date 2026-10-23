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

func (f kernelGuardFixture) controller(t *testing.T, mode string, container bool) (string, error) {
	t.Helper()
	root := filepath.Join(f.dir, "controller")
	for _, path := range []string{"deploy", "internal/provisioning"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	wrapper, err := os.ReadFile("../../deploy/host-memory.py")
	if err != nil {
		t.Fatal(err)
	}
	f.write(t, filepath.Join(root, "deploy/host-memory.py"), strings.ReplaceAll(string(wrapper), "/var/lib/datacenter-bot-health", filepath.Join(f.dir, "health")), 0600)
	replace := strings.NewReplacer("/var/lib/digital-ocean-bot-kernel-memory", f.root, "/etc/default/grub.d/99-dob-kho.cfg", f.drop, "/boot/grub/grub.cfg", f.grub, "/proc/sys/kernel/random/boot_id", f.boot, "/proc/cmdline", f.cmdline, "/proc/meminfo", f.mem, "/proc/sys/kernel/osrelease", filepath.Join(f.dir, "osrelease"), "/boot/config-", filepath.Join(f.dir, "config-"), "OWNER_UID=0", "OWNER_UID="+strconv.Itoa(os.Getuid()))
	f.write(t, filepath.Join(root, "internal/provisioning/kernel_memory_guard.py"), replace.Replace(kernelMemoryGuardPython), 0600)
	detect := "#!/bin/sh\nexit 1\n"
	if container {
		detect = "#!/bin/sh\necho lxc\nexit 0\n"
	}
	f.write(t, filepath.Join(f.dir, "bin/systemd-detect-virt"), detect, 0700)
	c := exec.Command("python3", filepath.Join(root, "deploy/host-memory.py"), mode)
	c.Env = append(os.Environ(), "PATH="+filepath.Join(f.dir, "bin")+":"+os.Getenv("PATH"))
	b, err := c.CombinedOutput()
	return string(b), err
}

func TestControllerKernelMemoryGuardLifecycle(t *testing.T) {
	f := newKernelGuardFixture(t)
	original, _ := os.ReadFile(f.grub)
	if out, err := f.controller(t, "check", false); err == nil || !strings.Contains(out, "kernel_memory_reboot_required") {
		t.Fatalf("unprotected controller accepted: %s %v", out, err)
	}
	after, _ := os.ReadFile(f.grub)
	if string(original) != string(after) {
		t.Fatal("read-only check changed boot configuration")
	}
	if _, err := os.Stat(f.root); !os.IsNotExist(err) {
		t.Fatal("preflight created a mitigation journal")
	}
	if out, err := f.controller(t, "prepare", false); err != nil {
		t.Fatal(out, err)
	}
	if out, err := f.controller(t, "check", false); err == nil {
		t.Fatal("prepared but inactive mitigation accepted", out)
	}
	f.write(t, f.boot, "boot-after\n", 0600)
	f.write(t, f.cmdline, "root=UUID=keep ro console=ttyS1 kho=off\n", 0600)
	f.write(t, f.mem, "MemTotal: 1675908 kB\nCmaTotal: 0 kB\nCmaFree: 0 kB\n", 0600)
	for _, mode := range []string{"check", "activation"} {
		out, err := f.controller(t, mode, false)
		if err != nil {
			t.Fatal(out, err)
		}
		var report map[string]any
		if json.Unmarshal([]byte(out), &report) != nil || report["status"] != "ok" {
			t.Fatal("missing activation proof", out)
		}
	}
	f.write(t, f.mem, "MemTotal: 1675908 kB\nCmaTotal: 0 kB\nCmaFree: 128 kB\n", 0600)
	if out, err := f.controller(t, "check", false); err == nil {
		t.Fatal("recurrent memory anomaly accepted", out)
	}
}

func TestControllerKernelMemoryGuardContainerBoundary(t *testing.T) {
	for _, container := range []bool{false, true} {
		f := newKernelGuardFixture(t)
		if !container {
			f.write(t, filepath.Join(f.dir, "osrelease"), "5.15.0-test\n", 0600)
			f.write(t, f.mem, "MemTotal: 2048000 kB\nCmaTotal: 0 kB\nCmaFree: 0 kB\n", 0600)
		}
		before, _ := os.ReadFile(f.grub)
		out, err := f.controller(t, "check", container)
		if err != nil {
			t.Fatal(out, err)
		}
		var report map[string]any
		if json.Unmarshal([]byte(out), &report) != nil {
			t.Fatal(out)
		}
		want := "ok"
		if container {
			want = "externally_managed"
		}
		if report["status"] != want {
			t.Fatal(out)
		}
		if out, err = f.controller(t, "prepare", container); err != nil {
			t.Fatal(out, err)
		}
		after, _ := os.ReadFile(f.grub)
		if string(before) != string(after) {
			t.Fatal("unaffected or externally-owned boot was changed")
		}
		if _, err := os.Stat(f.log); !os.IsNotExist(err) {
			t.Fatal("unexpected boot generator execution")
		}
	}
}
