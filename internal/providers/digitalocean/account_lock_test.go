package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"testing"
	"time"
)

func TestExplicitAccountLockHTTPMatrix(t *testing.T) {
	message := "There is currently a lock on the account, please log in to the control panel and contact support."
	for _, code := range []int{400, 403, 422} {
		h := HTTPError{Status: code, Code: "invalid_request", Message: message, RetryAfter: time.Minute}
		err := normalizeError("observe_inventory", fmt.Errorf("wrapped: %w", h))
		var p *providers.Error
		if !errors.As(err, &p) || p.Class != providers.ErrorAccountLocked || p.StatusCode != code || p.Operation != "observe_inventory" || p.RetryAfter != time.Minute || !errors.Is(err, h) {
			t.Fatalf("metadata/class lost: %+v", err)
		}
	}
	for _, tc := range []struct {
		code int
		msg  string
		want providers.ErrorClass
	}{
		{422, "Your account locked this volume during migration", providers.ErrorInvalidRequest},
		{422, "Your account is lockedness is unknown", providers.ErrorInvalidRequest},
		{422, "Your account has been locked resources list", providers.ErrorInvalidRequest},
		{403, "Your account is locked. Contact support.", providers.ErrorAccountLocked},
		{422, "Your team has been locked due to nonpayment", providers.ErrorAccountLocked},
		{401, message, providers.ErrorAuthentication}, {429, message, providers.ErrorRateLimited}, {503, message, providers.ErrorUnavailable},
		{422, "This droplet is locked", providers.ErrorInvalidRequest}, {400, "invalid region", providers.ErrorRegionCapacity},
		{403, "resource locked", providers.ErrorPermissionDenied}, {422, "contact support", providers.ErrorInvalidRequest},
	} {
		if got := providers.Class(normalizeError("test", HTTPError{Status: tc.code, Message: tc.msg})); got != tc.want {
			t.Fatalf("%+v got %s", tc, got)
		}
	}
}

func TestLockedAccountObservationPrecedesOtherReads(t *testing.T) {
	for _, mode := range []string{"full", "fast", "health"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := fixtureClient(t, func(r *http.Request) (int, string) {
				calls++
				if r.URL.Path != "/v2/account" {
					t.Fatalf("read after explicit lock: %s", r.URL.Path)
				}
				return 200, `{"account":{"uuid":"fixture","status":"locked","status_message":"Provider support required","droplet_limit":3}}`
			})
			d, _ := NewDriver(c)
			var err error
			switch mode {
			case "full":
				_, err = d.Observe(context.Background())
			case "fast":
				_, err = d.ObserveFast(context.Background())
			default:
				err = d.Health(context.Background())
			}
			if providers.Class(err) != providers.ErrorAccountLocked || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
	for _, status := range []string{"active", "warning", "trial_restricted", "unknown"} {
		if lockedAccountObservation("test", status, "locked wording alone") != nil {
			t.Fatalf("invented lock for %s", status)
		}
	}
}

func TestExplicitAccountLockCreateOutcome(t *testing.T) {
	for _, tc := range []struct {
		code    int
		message string
		class   providers.ErrorClass
		outcome providers.MutationOutcome
	}{
		{422, "There is currently a lock on the account, contact support", providers.ErrorAccountLocked, providers.OutcomeRejected},
		{422, "Your account locked this volume during migration", providers.ErrorInvalidRequest, providers.OutcomeRejected},
		{503, "There is currently a lock on the account", providers.ErrorUnavailable, providers.OutcomeAmbiguous},
	} {
		c := fixtureClient(t, func(r *http.Request) (int, string) {
			return tc.code, fmt.Sprintf(`{"id":"invalid_request","message":%q}`, tc.message)
		})
		d, _ := NewDriver(c)
		result, err := d.CreateServer(context.Background(), providers.CreateServerRequest{Name: "fixture", RegionID: "fra1", PlanID: "s", ImageID: "ubuntu"})
		if result.Outcome != tc.outcome || providers.Class(err) != tc.class {
			t.Fatal(result, err)
		}
	}
}
