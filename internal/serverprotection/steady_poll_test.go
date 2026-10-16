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
)

type steadySSH struct {
	inventory string
	calls     int
}

func (s *steadySSH) Run(_ context.Context, _ provisioning.Target, _ []byte, cmd string) (string, error) {
	s.calls++
	if strings.Contains(cmd, "uname -m") {
		return s.inventory, nil
	}
	return "", errors.New("unexpected installation during steady polling")
}
func (s *steadySSH) Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error {
	return errors.New("unexpected upload")
}
func TestSteadyProtectionPollDoesNotReinstall(t *testing.T) {
	p := Policy{Revision: 7, Enabled: true, PanelID: "11111111-1111-4111-8111-111111111111", ExcludedPorts: []int{22, 2053}}
	raw, _ := json.Marshal(p)
	data := []byte("guardian fixture")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server-guardian-linux-amd64"), data, 0600)
	st := Status{Version: Version, Revision: 7, Enabled: true, AgentRunning: true, NFTSupported: true, State: "READY", Ports: []int{443}}
	for _, state := range []string{"READY", "CRITICAL", "UNSUPPORTED"} {
		t.Run(state, func(t *testing.T) {
			x := st
			x.State = state
			x.AdmissionBlocked = state == "CRITICAL"
			if state == "UNSUPPORTED" {
				x.Ports = nil
			}
			b, _ := json.Marshal(x)
			ssh := &steadySSH{inventory: fmt.Sprintf("x86_64\n%x\n%x\n%x\nenabled\n%s\n", sha256.Sum256(data), sha256.Sum256(append(raw, '\n')), sha256.Sum256([]byte(Unit)), b)}
			got, e := (Controller{SSH: ssh, ArtifactDir: dir}).apply(context.Background(), nodeTarget{Panel: p.PanelID, Policy: p}, nil)
			if e != nil || ssh.calls != 1 || got.State != state {
				t.Fatalf("read-only receipt state=%s calls=%d err=%v", got.State, ssh.calls, e)
			}
		})
	}
}

func TestSteadyProtectionPollRejectsUnprovenInstallation(t *testing.T) {
	p := Policy{Revision: 7, Enabled: true, PanelID: "11111111-1111-4111-8111-111111111111"}
	data := []byte("guardian fixture")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server-guardian-linux-amd64"), data, 0600)
	raw, _ := json.Marshal(p)
	for _, bad := range []string{"policy", "unit", "disabled", "dead", "stale", "revision", "enabled", "malformed"} {
		t.Run(bad, func(t *testing.T) {
			st := Status{Version: Version, Revision: 7, Enabled: true, AgentRunning: true, NFTSupported: true, State: "READY", Ports: []int{443}}
			ph := fmt.Sprintf("%x", sha256.Sum256(append(raw, '\n')))
			uh := fmt.Sprintf("%x", sha256.Sum256([]byte(Unit)))
			enabled := "enabled"
			switch bad {
			case "policy":
				ph = "foreign"
			case "unit":
				uh = "foreign"
			case "disabled":
				enabled = "disabled"
			case "dead":
				st.AgentRunning = false
			case "stale":
				st.SampleAgeMS = 6000
			case "revision":
				st.Revision--
			case "enabled":
				st.Enabled = false
			}
			b, _ := json.Marshal(st)
			if bad == "malformed" {
				b = []byte("{")
			}
			ssh := &steadySSH{inventory: fmt.Sprintf("x86_64\n%x\n%s\n%s\n%s\n%s\n", sha256.Sum256(data), ph, uh, enabled, b)}
			_, err := (Controller{SSH: ssh, ArtifactDir: dir}).apply(context.Background(), nodeTarget{Panel: p.PanelID, Policy: p}, nil)
			if err == nil || ssh.calls != 2 {
				t.Fatalf("unproven observation accepted: calls=%d err=%v", ssh.calls, err)
			}
		})
	}
}
func TestCurrentAgentRejectsStoppedStartupReceipt(t *testing.T) {
	p := Policy{Revision: 3, Enabled: true}
	st := Status{Version: Version, Revision: 3, Enabled: true, AgentRunning: false, State: "READY"}
	if st.CurrentAgent(p) {
		t.Fatal("old sample after stopped/restarting agent was accepted")
	}
	st.AgentRunning = true
	if !st.CurrentAgent(p) {
		t.Fatal("current running sample rejected")
	}
	st.SampleAgeMS = -1
	if st.CurrentAgent(p) {
		t.Fatal("negative age accepted")
	}
}
