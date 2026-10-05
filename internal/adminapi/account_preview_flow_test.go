package adminapi

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingPreviewDriver struct{ stage string }

func (d failingPreviewDriver) Name() string                 { return "upcloud" }
func (d failingPreviewDriver) Health(context.Context) error { return nil }
func (d failingPreviewDriver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true}
}
func (d failingPreviewDriver) failure(stage string) error {
	if d.stage == stage {
		return &providers.Error{Class: providers.ErrorUnavailable, Operation: "fixture_" + stage, StatusCode: 200, Message: "must-not-leak"}
	}
	return nil
}
func (d failingPreviewDriver) Account(context.Context) (providers.Account, error) {
	return providers.Account{}, d.failure("account")
}
func (d failingPreviewDriver) Capacity(context.Context) (providers.Capacity, error) {
	return providers.Capacity{}, d.failure("capacity")
}
func (d failingPreviewDriver) Catalog(context.Context) (providers.Catalog, error) {
	return providers.Catalog{}, d.failure("catalog")
}

type failingPreviewFactory struct{ stage string }

func (f failingPreviewFactory) Name() string { return "upcloud" }
func (f failingPreviewFactory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "upcloud", Status: "ready"}
}
func (f failingPreviewFactory) Open(_ context.Context, r providers.OpenRequest) (providers.Driver, error) {
	return failingPreviewDriver{f.stage}, nil
}
func TestAccountPreviewReportsActualFailingStage(t *testing.T) {
	for _, stage := range []string{"account", "capacity", "catalog"} {
		t.Run(stage, func(t *testing.T) {
			registry := providers.NewRegistry()
			if err := registry.Register(failingPreviewFactory{stage}); err != nil {
				t.Fatal(err)
			}
			s := Server{Container: app.Container{Providers: registry}}
			req := httptest.NewRequest("POST", "/api/v1/accounts/preview", strings.NewReader(`{"provider":"upcloud","token":"ucat_fixture","proxy_id":""}`))
			req = req.WithContext(context.WithValue(req.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
			w := httptest.NewRecorder()
			s.accountPreview(w, req)
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != 502 || body["stage"] != stage || body["request_id"] == "" || body["provider_status"] != float64(200) {
				t.Fatal(w.Code, body)
			}
			if strings.Contains(w.Body.String(), "must-not-leak") || strings.Contains(w.Body.String(), "ucat_fixture") {
				t.Fatal("secret leaked")
			}
		})
	}
}
