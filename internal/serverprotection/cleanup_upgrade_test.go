package serverprotection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cleanupUpgradeSSH struct {
	inventory string
	cleanup   string
	uploads   int
	calls     int
}

func (s *cleanupUpgradeSSH) Run(_ context.Context, _ provisioning.Target, _ []byte, cmd string) (string, error) {
	s.calls++
	if strings.Contains(cmd, "uname -m") {
		return s.inventory, nil
	}
	if strings.Contains(cmd, "mktemp") {
		return "/var/tmp/dob-guardian.fixture", nil
	}
	if strings.Contains(cmd, " cleanup\n") {
		s.cleanup = cmd
		raw, _ := json.Marshal(Status{Version: Version, Revision: 8, State: "DISABLED", ObservedAt: time.Now()})
		return string(raw), nil
	}
	if strings.HasPrefix(cmd, "rm -f") {
		return "", nil
	}
	return "", errors.New("unexpected command")
}
func (s *cleanupUpgradeSSH) Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error {
	s.uploads++
	return errors.New("fixture upload unavailable")
}
func TestDisabledCleanupDoesNotWaitForArtifactUpgrade(t *testing.T) {
	p := Policy{Revision: 8, PanelID: "11111111-1111-4111-8111-111111111111"}
	old := Status{Version: Version, Revision: 7, Enabled: true, AgentRunning: true, AgentPID: 42, State: "CRITICAL", AdmissionBlocked: true}
	raw, _ := json.Marshal(old)
	transport := &cleanupUpgradeSSH{inventory: fmt.Sprintf("x86_64\n%x\nold-policy\n%x\nenabled\n42\n%s", sha256.Sum256([]byte("previous-release")), sha256.Sum256([]byte(Unit)), raw)}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server-guardian-linux-amd64"), []byte("new-release"), 0600)
	got, err := (Controller{SSH: transport, ArtifactDir: dir}).apply(context.Background(), nodeTarget{Policy: p}, nil)
	if err != nil || got.State != "DISABLED" || transport.uploads != 0 || transport.calls != 2 {
		t.Fatalf("cleanup blocked on update: status=%s uploads=%d calls=%d err=%v", got.State, transport.uploads, transport.calls, err)
	}
	if err = got.ValidateReceipt(p); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"foreign guardian installation", "unsafe guardian symlink", " configure", "systemctl stop", " cleanup\n"} {
		if !strings.Contains(transport.cleanup, required) {
			t.Fatal("lost cleanup guard", required)
		}
	}
	if strings.Contains(transport.cleanup, "install -m") {
		t.Fatal("cleanup changed binary")
	}
}
func TestArtifactUpgradeStillRequiredForEnabledOrUnprovenNodes(t *testing.T) {
	for _, bad := range []string{"enabled", "unknown-version", "missing", "foreign-unit", "malformed", "invalid-hash"} {
		t.Run(bad, func(t *testing.T) {
			p := Policy{Revision: 8, PanelID: "11111111-1111-4111-8111-111111111111"}
			status := Status{Version: Version, Revision: 7, State: "DISABLED"}
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte("old")))
			unit := fmt.Sprintf("%x", sha256.Sum256([]byte(Unit)))
			if bad == "enabled" {
				p.Enabled = true
			}
			if bad == "unknown-version" {
				status.Version = "0.0"
			}
			if bad == "missing" {
				hash = "missing"
			}
			if bad == "foreign-unit" {
				unit = "unknown"
			}
			if bad == "invalid-hash" {
				hash = strings.Repeat("z", 64)
			}
			raw, _ := json.Marshal(status)
			if bad == "malformed" {
				raw = []byte("{")
			}
			transport := &cleanupUpgradeSSH{inventory: fmt.Sprintf("x86_64\n%s\nold-policy\n%s\ndisabled\n0\n%s", hash, unit, raw)}
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "server-guardian-linux-amd64"), []byte("new"), 0600)
			_, err := (Controller{SSH: transport, ArtifactDir: dir}).apply(context.Background(), nodeTarget{Policy: p}, nil)
			if err == nil || transport.uploads != 1 || transport.cleanup != "" {
				t.Fatalf("upgrade bypass: %s uploads=%d err=%v", bad, transport.uploads, err)
			}
		})
	}
}
