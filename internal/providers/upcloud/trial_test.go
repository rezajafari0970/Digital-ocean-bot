package upcloud

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"testing"
)

func TestTrialAccountFlagAndCreateCode(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		fail      bool
	}{{"1", "trial_restricted", false}, {"0", "active", false}, {`"1"`, "trial_restricted", false}, {"null", "active", false}, {"2", "", true}, {`"garbage"`, "", true}} {
		t.Run(tc.raw, func(t *testing.T) {
			d := fixture(t, func(r *http.Request) (int, any, error) {
				var body any
				json.Unmarshal([]byte(`{"account":{"username":"fixture","trial_mode":`+tc.raw+`}}`), &body)
				return 200, body, nil
			}, nil)
			a, e := d.Account(context.Background())
			if (e != nil) != tc.fail || (!tc.fail && a.Status != tc.want) {
				t.Fatal(a, e)
			}
		})
	}
	e := normalize("create_server", apiError{status: 403, code: "TRIAL_FIREWALL"})
	var pe *providers.Error
	if !errors.As(e, &pe) || pe.Code != "TRIAL_FIREWALL" || pe.StatusCode != 403 || pe.Class != providers.ErrorPermissionDenied || providers.IsRetryable(e) {
		t.Fatal(e)
	}
}
