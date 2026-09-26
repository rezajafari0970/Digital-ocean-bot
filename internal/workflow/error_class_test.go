package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"testing"
)

func TestClassifyStepError(t *testing.T) {
	cases := []struct {
		name, step string
		err        error
		want       ErrorClass
	}{{"timeout", "provision", context.DeadlineExceeded, ErrorTimeout}, {"ssh_wait", "provision", provisioning.ErrSSHNotReady, ErrorRetryable}, {"ssh_command", "provision", provisioning.ErrSSHCommand, ErrorRetryable}, {"auth", "create", digitalocean.HTTPError{Status: 401}, ErrorAuth}, {"capacity", "create", digitalocean.HTTPError{Status: 422, Message: "region unavailable"}, ErrorCapacity}, {"server", "create", digitalocean.HTTPError{Status: 503}, ErrorRetryable}, {"badplan", "provision", provisioning.ErrInvalidPlan, ErrorPermanent}, {"provision_limit", "provision", provisioning.ErrStepRetryLimit, ErrorPermanent}, {"unknown", "panel", errors.New("x"), ErrorUnknown}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyStepError(c.step, c.err); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}
