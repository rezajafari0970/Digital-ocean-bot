package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

const fixtureBootID = "11111111-1111-4111-8111-111111111111"

type bootstrapRebootSSH struct {
	reads, sends int
	fail         bool
	commands     []string
}

func (s *bootstrapRebootSSH) Wait(context.Context, provisioning.Target, []byte) error { return nil }
func (s *bootstrapRebootSSH) Run(_ context.Context, _ provisioning.Target, _ []byte, cmd string) (string, error) {
	s.commands = append(s.commands, cmd)
	if cmd == "cat /proc/sys/kernel/random/boot_id" {
		s.reads++
		return fixtureBootID + "\n", nil
	}
	s.sends++
	if s.fail {
		return "", errors.New("lost SSH response")
	}
	return "", nil
}
func TestInstallerBootstrapRebootCommandBootFence(t *testing.T) {
	dir := t.TempDir()
	boot := filepath.Join(dir, "boot")
	calls := filepath.Join(dir, "calls")
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\necho called >> \"$REBOOT_TEST_CALLS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{fixtureBootID, "22222222-2222-4222-8222-222222222222"} {
		if err := os.WriteFile(boot, []byte(id+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", "-c", strings.ReplaceAll(installerRebootCommand(fixtureBootID), "/proc/sys/kernel/random/boot_id", boot))
		cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "REBOOT_TEST_CALLS="+calls)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run%d: %v %s", i, err, out)
		}
	}
	b, err := os.ReadFile(calls)
	if err != nil || string(b) != "called\n" {
		t.Fatalf("rebooted subsequent boot: %q %v", b, err)
	}
	if installerRebootBootID("boot_id=invalid") != "" {
		t.Fatal("invalid boot accepted")
	}
}
