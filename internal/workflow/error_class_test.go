package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"testing"
)

func TestClassifyStepError(t *testing.T) {
	cases := []struct {
		name, step string
		err        error
		want       ErrorClass
	}{{"timeout", "provision", context.DeadlineExceeded, ErrorTimeout}, {"ssh_wait", "provision", provisioning.ErrSSHNotReady, ErrorRetryable}, {"ssh_command", "provision", provisioning.ErrSSHCommand, ErrorRetryable}, {"auth", "create", &providers.Error{Class: providers.ErrorAuthentication}, ErrorAuth}, {"capacity", "create", &providers.Error{Class: providers.ErrorRegionCapacity}, ErrorCapacity}, {"server", "create", &providers.Error{Class: providers.ErrorUnavailable}, ErrorRetryable}, {"badplan", "provision", provisioning.ErrInvalidPlan, ErrorPermanent}, {"provision_limit", "provision", provisioning.ErrStepRetryLimit, ErrorPermanent}, {"missing_ssh_secret", "provision", secrets.ErrSecretNotFound, ErrorDependency}, {"unknown", "panel", errors.New("x"), ErrorUnknown}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyStepError(c.step, c.err); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}
