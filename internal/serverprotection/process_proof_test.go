package serverprotection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestExecutingArtifactRejectsReplacedPathAndOldSample(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sleep")
	sleep, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, sleep, 0700); err != nil {
		t.Fatal(err)
	}
	process := exec.Command(path, "30")
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { process.Process.Kill(); process.Wait() }()
	live, err := observeProcess("/proc", process.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if live.SHA256 != fmt.Sprintf("%x", sha256.Sum256(sleep)) {
		t.Fatal("wrong executing artifact")
	}
	replacement, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	next := path + ".next"
	if err = os.WriteFile(next, replacement, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(next, path); err != nil {
		t.Fatal(err)
	}
	after, err := observeProcess("/proc", process.Process.Pid)
	if err != nil || after != live {
		t.Fatal("path replacement changed proof of running inode", after, err)
	}
	expected := fmt.Sprintf("%x", sha256.Sum256(replacement))
	if expected == live.SHA256 {
		t.Fatal("fixture artifacts identical")
	}
	p := Policy{Revision: 7, Enabled: true, PanelID: "11111111-1111-4111-8111-111111111111", ExcludedPorts: []int{22, 2053}}
	st := Status{Version: Version, Revision: 7, Enabled: true, AgentRunning: true, AgentPID: live.PID, AgentStartTicks: live.StartTicks, AgentBinarySHA256: live.SHA256, State: "READY", NFTSupported: true, Ports: []int{443}}
	raw, _ := json.Marshal(p)
	sample, _ := json.Marshal(st)
	inventory := fmt.Sprintf("x86_64\n%s\n%x\n%x\nenabled\n%d\n%s\n", expected, sha256.Sum256(append(raw, '\n')), sha256.Sum256([]byte(Unit)), live.PID, sample)
	if err = os.WriteFile(filepath.Join(dir, "server-guardian-linux-amd64"), replacement, 0600); err != nil {
		t.Fatal(err)
	}
	ssh := &steadySSH{inventory: inventory}
	_, err = (Controller{SSH: ssh, ArtifactDir: dir}).apply(context.Background(), nodeTarget{Panel: p.PanelID, Policy: p}, nil)
	if err == nil || ssh.calls != 1 {
		t.Fatal("partial activation accepted or blindly retried", ssh.calls, err)
	}
	old := live
	old.StartTicks += "0"
	if _, err = verifyProducer(old); err == nil {
		t.Fatal("PID reuse / old writer accepted")
	}
	if _, err = verifyProducer(processIdentity{}); err == nil {
		t.Fatal("legacy unknown producer attested")
	}
	process.Process.Kill()
	process.Wait()
	if _, err = observeProcess("/proc", live.PID); err == nil {
		t.Fatal("exited process attested")
	}
}
func TestProcessProofRejectsMissingAndMalformedKernelEvidence(t *testing.T) {
	root := t.TempDir()
	for _, pid := range []int{0, -1, 42} {
		if _, err := observeProcess(root, pid); err == nil {
			t.Fatal("absent process accepted", pid)
		}
	}
	path := filepath.Join(root, strconv.Itoa(42))
	os.Mkdir(path, 0700)
	os.WriteFile(filepath.Join(path, "stat"), []byte("42 (broken) S"), 0600)
	os.WriteFile(filepath.Join(path, "exe"), []byte("fixture"), 0600)
	if _, err := observeProcess(root, 42); err == nil {
		t.Fatal("truncated stat accepted")
	}
}
