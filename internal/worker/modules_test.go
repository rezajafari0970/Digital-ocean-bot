package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestModuleRolesSupervisionAndJoinedShutdown(t *testing.T) {
	if _, err := ParseRole("panel"); err == nil {
		t.Fatal("unknown role accepted")
	}
	if RoleControl.ConnectionBudget()+RolePanels.ConnectionBudget() != RoleAll.ConnectionBudget() {
		t.Fatal("DB budget grew")
	}
	var modules Modules
	started := make(chan struct{})
	released := make(chan struct{})
	var wrong atomic.Bool
	modules.Add(RolePanels, "native-panel", func(ctx context.Context) { close(started); <-ctx.Done(); <-released })
	modules.Add(RoleControl, "cloud", func(context.Context) { wrong.Store(true) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- modules.Run(ctx, RolePanels) }()
	<-started
	cancel()
	select {
	case <-done:
		t.Fatal("shutdown abandoned live handler")
	case <-time.After(20 * time.Millisecond):
	}
	close(released)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if wrong.Load() {
		t.Fatal("unselected role ran")
	}
	var failed Modules
	peerCanceled := make(chan struct{})
	failed.Add(RoleControl, "unexpected-return", func(context.Context) {})
	failed.Add(RoleControl, "peer", func(ctx context.Context) { <-ctx.Done(); close(peerCanceled) })
	if err := failed.Run(context.Background(), RoleControl); err == nil {
		t.Fatal("silent stopped module")
	}
	select {
	case <-peerCanceled:
	case <-time.After(time.Second):
		t.Fatal("peer not canceled")
	}
}
func TestDuplicateModuleFailsBeforeStarting(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate module accepted")
		}
	}()
	var m Modules
	m.Add(RoleControl, "same", func(context.Context) {})
	m.Add(RolePanels, "same", func(context.Context) {})
}

// Real independent OS processes: one role crashes; its peer keeps advancing;
// a separately started replacement advances again. This does not touch a
// production service, provider, panel, database, or execution gate.
func TestModuleProcessFaultIsolation(t *testing.T) {
	if role := os.Getenv("DOB_MODULE_TEST_ROLE"); role != "" {
		var m Modules
		m.Add(Role(role), "fixture", func(ctx context.Context) {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			n := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				n++
				_ = os.WriteFile(filepath.Join(os.Getenv("DOB_MODULE_TEST_DIR"), role), []byte(strings.Repeat("x", n)), 0600)
				if _, err := os.Stat(filepath.Join(os.Getenv("DOB_MODULE_TEST_DIR"), role+"-crash")); err == nil {
					panic("fixture crash")
				}
			}
		})
		if err := m.Run(context.Background(), Role(role)); err != nil {
			os.Exit(12)
		}
		return
	}
	dir := t.TempDir()
	spawn := func(role string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestModuleProcessFaultIsolation$")
		cmd.Env = append(os.Environ(), "DOB_MODULE_TEST_ROLE="+role, "DOB_MODULE_TEST_DIR="+dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	size := func(role string) int { b, _ := os.ReadFile(filepath.Join(dir, role)); return len(b) }
	until := func(f func() bool) {
		t.Helper()
		end := time.Now().Add(4 * time.Second)
		for time.Now().Before(end) {
			if f() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("process condition timed out")
	}
	control := spawn("control")
	panels := spawn("panels")
	until(func() bool { return size("control") > 2 && size("panels") > 2 })
	if err := os.WriteFile(filepath.Join(dir, "control-crash"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := control.Wait(); err == nil {
		t.Fatal("crash returned success")
	}
	prior := size("panels")
	until(func() bool { return size("panels") > prior+2 })
	if err := os.Remove(filepath.Join(dir, "control-crash")); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "control"))
	restarted := spawn("control")
	until(func() bool { return size("control") > 2 })
	_ = panels.Process.Kill()
	_ = restarted.Process.Kill()
}
