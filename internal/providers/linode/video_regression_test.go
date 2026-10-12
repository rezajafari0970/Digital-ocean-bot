package linode

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"testing"
)

func TestDeploymentLabelsUniqueStableAndLegacyTagAdoption(t *testing.T) {
	req := request(t)
	req.Name = "account-auto"
	var labels []string
	var d *Driver
	d = driver(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v4/account/settings" {
			return response(200, `{"backups_enabled":false,"managed":false}`), nil
		}
		if r.Method == http.MethodPost {
			var x createRequest
			if e := json.NewDecoder(r.Body).Decode(&x); e != nil {
				t.Fatal(e)
			}
			labels = append(labels, x.Label)
			if !contains(x.Tags, identityTag(req.Identity)) || !contains(x.Tags, d.ownerTag()) {
				t.Fatal("ownership changed")
			}
			return response(200, server(7, nil)), nil
		}
		// Older ambiguous create retains its original non-suffixed label; identity tags adopt it.
		return response(200, page("["+server(7, []string{d.ownerTag(), "managed-by-digital-ocean-bot", identityTag(req.Identity)})+"]", 1, 1, 1)), nil
	})
	for i := 0; i < 2; i++ {
		if _, e := d.CreateServer(context.Background(), req); e != nil {
			t.Fatal(e)
		}
	}
	if labels[0] != labels[1] || labels[0] == req.Name {
		t.Fatal(labels)
	}
	req.Identity = "dob-deployment-33333333-3333-4333-8333-333333333333"
	if _, e := d.CreateServer(context.Background(), req); e != nil {
		t.Fatal(e)
	}
	if labels[0] == labels[2] {
		t.Fatal("same-profile deployments collide")
	}
	for _, name := range []string{"abc", strings.Repeat("a", 64), strings.Repeat("a", 30) + "-b"} {
		label := d.deploymentLabel(name, req.Identity)
		if !labelPattern.MatchString(label) || len(label) > 64 || strings.Contains(label, "--") {
			t.Fatal(label)
		}
	}
	oldAccount := d.accountID
	before := d.deploymentLabel(req.Name, req.Identity)
	d.accountID = "other-account"
	if before == d.deploymentLabel(req.Name, req.Identity) {
		t.Fatal("account omitted from label")
	}
	d.accountID = oldAccount
	matches, e := d.FindServerByIdentity(context.Background(), req.Identity)
	if e != nil || len(matches) != 1 || len(labels) != 3 {
		t.Fatal("legacy adoption issued create", matches, e)
	}
}
func TestCreateRejectsConflictingTagsAndControlIdentityBeforeEgress(t *testing.T) {
	d := driver(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid ownership reached provider")
		return nil, nil
	})
	for _, tag := range []string{"dob-account-foreign", "dob-dep-foreign", "dob-id-foreign"} {
		req := request(t)
		req.Tags = append(req.Tags, tag)
		res, e := d.CreateServer(context.Background(), req)
		if !providers.IsClass(e, providers.ErrorInvalidRequest) || res.Outcome != providers.OutcomeRejected {
			t.Fatal(res, e)
		}
	}
	req := request(t)
	req.Identity = strings.Repeat("x", 60) + "\x00"
	if _, e := d.CreateServer(context.Background(), req); e == nil {
		t.Fatal("invalid identity")
	}
}
func TestValidationDiagnosticsBoundedAndNotRetryable(t *testing.T) {
	for _, tc := range []struct {
		body, code string
		class      providers.ErrorClass
	}{
		{`{"errors":[{"field":"label","reason":"label must be unique; secret-value"}]}`, "LABEL_CONFLICT", providers.ErrorInvalidRequest},
		{`{"errors":[{"field":"type","reason":"secret-value"}]}`, "INVALID_TYPE", providers.ErrorInvalidRequest},
		{`{"errors":[{"field":"secret-value","reason":"secret-value"}]}`, "REQUEST_REJECTED", providers.ErrorInvalidRequest},
		{`{"errors":[{"field":"label","reason":"unique"},{"field":"x","reason":"Account limit reached secret-value"}]}`, "ACCOUNT_LIMIT_REACHED", providers.ErrorCapacity},
	} {
		e := classify("create", 400, []byte(tc.body))
		if e.Code != tc.code || e.Class != tc.class || strings.Contains(e.Error(), "secret-value") {
			t.Fatal(e)
		}
	}
}
