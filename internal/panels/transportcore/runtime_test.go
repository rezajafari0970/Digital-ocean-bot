package transportcore

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"os"
	"strings"
	"testing"
)

type failedUpload struct {
	remote, local string
	cleaned       bool
	cancel        context.CancelFunc
}

func (s *failedUpload) Upload(ctx context.Context, _ provisioning.Target, _ []byte, local, remote string, mode os.FileMode) error {
	s.local, s.remote = local, remote
	if mode.Perm() != 0600 {
		return errors.New("unsafe mode")
	}
	s.cancel()
	return errors.New("partial upload")
}
func (s *failedUpload) Run(ctx context.Context, _ provisioning.Target, _ []byte, cmd string) (string, error) {
	s.cleaned = ctx.Err() == nil && cmd == "rm -f -- '"+s.remote+"'"
	return "", nil
}
func TestPartialUploadIsCleanedAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ssh := &failedUpload{cancel: cancel}
	err := Prepare(ctx, ssh, provisioning.Target{}, nil, realityconfig.Payload{StreamSettings: map[string]any{"network": "xhttp"}})
	if err == nil || !ssh.cleaned || !strings.HasPrefix(ssh.remote, "/root/.dob-transport-check-") {
		t.Fatal("partial secret file not cleaned")
	}
	if _, err = os.Stat(ssh.local); !os.IsNotExist(err) {
		t.Fatal("local candidate retained")
	}
}
