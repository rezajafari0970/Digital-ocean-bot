package digitalocean

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProviderWarningMessageSurvivesAccountAndObservation(t *testing.T) {
	message := "Verify your email before creating resources."
	client, err := NewClient(accounts.NewContext("fixture"), "token", secretStub{[]byte("fixture")}, &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"regions":[],"sizes":[],"images":[]}`
		if strings.HasSuffix(r.URL.Path, "/account") {
			raw, _ := json.Marshal(map[string]any{"account": map[string]any{"uuid": "fixture", "status": "warning", "status_message": message, "droplet_limit": 3}})
			body = string(raw)
		}
		if strings.HasSuffix(r.URL.Path, "/droplets") {
			body = `{"droplets":[]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	driver, _ := NewDriver(client)
	a, err := driver.Account(context.Background())
	if err != nil || a.Status != "warning" || a.StatusMessage != message {
		t.Fatal(a, err)
	}
	for _, read := range []func(context.Context) (providers.Observation, error){driver.Observe, driver.ObserveFast} {
		o, err := read(context.Background())
		if err != nil || o.Account.Status != "warning" || o.Account.StatusMessage != message {
			t.Fatal(o.Account, err)
		}
	}
	message = "\n" + strings.Repeat("ا", 600) + "\r"
	a, err = driver.Account(context.Background())
	if err != nil || len([]rune(a.StatusMessage)) != 513 || strings.ContainsAny(a.StatusMessage, "\r\n") {
		t.Fatal("unbounded provider text", err)
	}
}
