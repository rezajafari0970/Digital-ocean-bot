package provisioning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type readinessSSHStub struct{ dnsFail bool }

func (s readinessSSHStub) RunDetailedObserved(_ context.Context, _ Target, _ []byte, cmd string, _ StageObserver) (CommandResult, error) {
	out := "ok"
	switch {
	case strings.Contains(cmd, "/etc/os-release"):
		out = "ubuntu|24.04"
	case strings.Contains(cmd, "uname -m"):
		out = "x86_64"
	case strings.Contains(cmd, "_NPROCESSORS"):
		out = "2"
	case strings.Contains(cmd, "MemTotal"):
		out = "2048"
	case strings.Contains(cmd, "df -Pm"):
		out = "20000"
	case cmd == "id -u":
		out = "0"
	case strings.Contains(cmd, "for x in apt-get"):
		out = "apt-get"
	case strings.Contains(cmd, "dpkg --audit"):
		out = "ok"
	case strings.Contains(cmd, "fuser /var/lib/dpkg"):
		out = "free"
	case strings.Contains(cmd, "getent hosts"):
		if s.dnsFail {
			code := 2
			return CommandResult{ExitCode: &code, Stderr: "Temporary failure resolving"}, fmt.Errorf("%w: exit 2", ErrSSHCommand)
		}
		out = "ok"
	case strings.Contains(cmd, "curl -fsSIL"):
		out = "ok"
	case strings.Contains(cmd, "NTPSynchronized"):
		out = "yes"
	case strings.Contains(cmd, "reboot-required"):
		out = "no"
	}
	code := 0
	return CommandResult{Stdout: out, ExitCode: &code}, nil
}
func TestReadinessReady(t *testing.T) {
	c := ReadinessCollector{SSH: readinessSSHStub{}}
	s, err := c.Collect(context.Background(), "r", Target{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "READY" || s.OSID != "ubuntu" || s.DiskFreeMB != 20000 || !s.DNSOK {
		t.Fatalf("%+v", s)
	}
}
func TestReadinessDNSBlocked(t *testing.T) {
	c := ReadinessCollector{SSH: readinessSSHStub{dnsFail: true}}
	s, err := c.Collect(context.Background(), "r", Target{}, nil, nil)
	if !errors.Is(err, ErrServerReadinessBlocked) {
		t.Fatalf("err=%v", err)
	}
	if s.Status != "BLOCKED" || s.DNSOK {
		t.Fatalf("%+v", s)
	}
}
