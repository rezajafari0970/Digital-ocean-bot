package transportcore

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"strings"
	"testing"
)

type currentSSH struct {
	receipt string
	fail    bool
	cleanup bool
	command string
}

func (s *currentSSH) Run(ctx context.Context, _ provisioning.Target, _ []byte, command string) (string, error) {
	if strings.HasPrefix(command, "rm -f -- ") {
		s.cleanup = ctx.Err() == nil
		return "", nil
	}
	s.command = command
	if s.fail {
		return "", errors.New("disconnected")
	}
	return s.receipt, nil
}
func TestCurrentRuntimeRequiresCompleteReceiptAndCleans(t *testing.T) {
	for _, tt := range []struct {
		receipt     string
		fail, valid bool
	}{
		{`{"validated":true,"installed":true}`, false, true},
		{`{"validated":true,"installed":false}`, false, false},
		{`{"validated":true}`, false, false},
		{`prefix {"validated":true,"installed":true}`, false, false},
		{`{"validated":true,"installed":true}`, true, false},
	} {
		ssh := &currentSSH{receipt: tt.receipt, fail: tt.fail}
		err := PrepareCurrent(context.Background(), ssh, provisioning.Target{}, nil)
		if (err == nil) != tt.valid || !ssh.cleanup {
			t.Fatalf("receipt=%q err=%v cleanup=%v", tt.receipt, err, ssh.cleanup)
		}
		for _, required := range []string{"O_EXCL", "0o600", "/usr/local/x-ui/bin/config.json", "finally:", "os.unlink"} {
			if !strings.Contains(ssh.command, required) {
				t.Fatal("missing secret handling", required)
			}
		}
	}
}
