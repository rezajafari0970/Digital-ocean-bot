package serverprotection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAgentNamespaceLifecycleFault(t *testing.T) {
	if os.Getenv("DOB_AGENT_RUN_CHILD") == "1" {
		if e := RunAgent(context.Background()); e != nil {
			t.Fatal(e)
		}
		return
	}
	if os.Getenv("DOB_AGENT_NAMESPACE_CHILD") != "1" {
		if os.Getenv("DOB_RUN_NFT_TEST") != "1" {
			t.Skip("isolated agent lifecycle opt-in")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "unshare", "--mount", "--net", "--propagation", "private", os.Args[0], "-test.run=^TestAgentNamespaceLifecycleFault$", "-test.v")
		cmd.Env = append(os.Environ(), "DOB_AGENT_NAMESPACE_CHILD=1")
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("agent sandbox %v %s", e, out)
		}
		t.Log(string(out))
		return
	}
	for _, ns := range []string{"net", "mnt"} {
		self, _ := os.Readlink("/proc/self/ns/" + ns)
		host, _ := os.Readlink("/proc/1/ns/" + ns)
		if self == host || self == "" {
			t.Fatal("unsafe namespace", ns)
		}
	}
	// Bind private scratch directories over all paths the agent can mutate.
	for _, target := range []string{"/etc", "/run", "/var/lib", "/usr/local"} {
		tmp := t.TempDir()
		if out, e := exec.Command("mount", "--bind", tmp, target).CombinedOutput(); e != nil {
			t.Fatalf("bind %s %v %s", target, e, out)
		}
	}
	os.MkdirAll(filepath.Dir(XrayConfig), 0700)
	os.WriteFile(XrayConfig, []byte(`{"inbounds":[{"protocol":"vless","port":14443}]}`), 0600)
	p := Policy{Revision: 1, Enabled: true, PanelID: "11111111-1111-4111-8111-111111111111", ExcludedPorts: []int{22, 2053}}
	configure := func(p Policy) error { raw, _ := json.Marshal(p); return Configure(strings.NewReader(string(raw))) }
	if e := configure(p); e != nil {
		t.Fatal(e)
	}
	proc := exec.Command(os.Args[0], "-test.run=^TestAgentNamespaceLifecycleFault$")
	proc.Env = append(os.Environ(), "DOB_AGENT_RUN_CHILD=1")
	if e := proc.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { proc.Process.Kill(); proc.Wait() }()
	wait := func() Status {
		t.Helper()
		end := time.Now().Add(5 * time.Second)
		for time.Now().Before(end) {
			s, e := ReadStatus()
			if e == nil && s.ValidateReceipt(p) == nil {
				return s
			}
			time.Sleep(50 * time.Millisecond)
		}
		s, e := ReadStatus()
		t.Fatalf("agent receipt unavailable %+v %v", s, e)
		return s
	}
	_ = wait()
	if e := RunAgent(context.Background()); !errors.Is(e, syscall.EWOULDBLOCK) {
		t.Fatal("duplicate agent lock failed", e)
	}
	lower := p
	lower.Revision = 0
	if configure(lower) == nil {
		t.Fatal("old policy accepted")
	}
	clash := p
	clash.Enabled = false
	if configure(clash) == nil {
		t.Fatal("same revision changed")
	}
	// Abrupt death must be reported as not running, independent of fresh status.
	proc.Process.Kill()
	proc.Wait()
	s, e := ReadStatus()
	if e != nil || s.AgentRunning || s.ValidateReceipt(p) == nil {
		t.Fatal("dead agent accepted", s, e)
	}
	n, e := nftBinary()
	if e != nil {
		t.Fatal(e)
	}
	if e = n.Apply(context.Background(), []int{14443}, true); e != nil {
		t.Fatal(e)
	}
	p.Revision++
	p.Enabled = false
	if e = configure(p); e != nil {
		t.Fatal(e)
	}
	if e = Cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	s, e = ReadStatus()
	if e != nil || s.ValidateReceipt(p) != nil {
		t.Fatal("disable not proven", s, e)
	}
	if present, e := n.present(context.Background()); e != nil || present {
		t.Fatal("owned nft rules survived", present, e)
	}
	// Exercise recovery boundaries using a deterministic systemctl fixture.
	os.MkdirAll("/etc/x-ui", 0700)
	os.WriteFile("/etc/x-ui/x-ui.db", []byte("fixture"), 0600)
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\nshow) cat /run/fixture-state;;\nstart) echo start >> /run/fixture-starts;;\n*) exit 1;;\nesac\n"
	os.WriteFile(bin+"/systemctl", []byte(script), 0700)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	p.Revision++
	p.Enabled = true
	if e = configure(p); e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"active", "activating", "deactivating"} {
		os.WriteFile("/run/fixture-state", []byte(state), 0600)
		checkRecovery(context.Background(), true)
	}
	if _, e = os.Stat("/run/fixture-starts"); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("active service started")
	}
	os.WriteFile("/run/fixture-state", []byte("inactive"), 0600)
	checkRecovery(context.Background(), false)
	_, detail := checkRecovery(context.Background(), true)
	calls, _ := os.ReadFile("/run/fixture-starts")
	if string(calls) != "start\n" {
		t.Fatal("inactive not started once", detail, string(calls))
	}
	checkRecovery(context.Background(), true)
	calls, _ = os.ReadFile("/run/fixture-starts")
	if string(calls) != "start\n" {
		t.Fatal("cooldown bypassed")
	}
	now := time.Now().Unix()
	if e = atomicJSON(StateDir+"/restarts.json", []int64{now - 240, now - 180, now - 120}); e != nil {
		t.Fatal(e)
	}
	checkRecovery(context.Background(), true)
	calls, _ = os.ReadFile("/run/fixture-starts")
	if string(calls) != "start\n" {
		t.Fatal("restart budget bypassed")
	}
	p.Revision++
	p.Enabled = false
	configure(p)
	os.Remove(StateDir + "/restarts.json")
	checkRecovery(context.Background(), true)
	calls, _ = os.ReadFile("/run/fixture-starts")
	if string(calls) != "start\n" {
		t.Fatal("disable policy bypassed")
	}
	// Execute the exact emitted installer, including unit-only update and disable.
	os.WriteFile(bin+"/systemctl", []byte("#!/bin/sh\necho \"$1\" >> /run/install-calls\n"), 0700)
	fixture := []byte("#!/bin/sh\ncase \"$1\" in\nconfigure) cat > /etc/dob-server-guardian/policy.json;;\nwait-status|status) cat /etc/dob-server-guardian/policy.json;;\ncleanup) true;;\n*) exit 1;;\nesac\n")
	stage := t.TempDir() + "/payload"
	os.WriteFile(stage, fixture, 0600)
	hash := fmt.Sprintf("%x", sha256.Sum256(fixture))
	p.Revision++
	p.Enabled = true
	install := func(stage string) {
		t.Helper()
		out, e := exec.Command("sh", "-c", installCommand(p, stage, hash)).CombinedOutput()
		if e != nil {
			t.Fatalf("actual installer %v %s", e, out)
		}
	}
	os.MkdirAll("/etc/systemd/system", 0755)
	install(stage)
	unitPath := "/etc/systemd/system/" + UnitName
	// /etc is private. Normally systemd supplies this directory.
	raw, e := os.ReadFile(unitPath)
	if e != nil || string(raw) != Unit {
		t.Fatal("unit not installed", e)
	}
	os.WriteFile(unitPath, append(raw, []byte("\n# prior unit version\n")...), 0600)
	install("")
	calls, _ = os.ReadFile("/run/install-calls")
	if strings.Count(string(calls), "restart\n") != 2 {
		t.Fatal("unit-only restart or duplicate restart", string(calls))
	}
	p.Revision++
	p.Enabled = false
	install("")
	calls, _ = os.ReadFile("/run/install-calls")
	if !strings.Contains(string(calls), "disable\n") {
		t.Fatal("disable unit missing")
	}
	t.Log("AGENT_ACTUAL_START_SINGLETON_CRASH_DETECTION_POLICY_FENCING_DISABLE_CLEANUP_BOUNDED_RECOVERY_INSTALLER PASS")
}
