package upcloud

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"testing"
)

func TestTrialCompatibleCreatePreservesFirewallAndIPv4(t *testing.T) {
	for _, deny := range []bool{false, true} {
		posts := 0
		d := fixture(t, func(r *http.Request) (int, any, error) {
			switch r.URL.Path {
			case "/1.3/plan":
				return 200, planJSON(), nil
			case "/1.3/storage/" + imageID:
				return 200, templateJSON(), nil
			case "/1.3/server":
				if r.Method != "POST" {
					t.Fatalf("unexpected method %s", r.Method)
				}
				posts++
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				b := body["server"]
				raw, _ := json.Marshal(b)
				if b["firewall"] != "on" || strings.Contains(string(raw), "IPv6") || b["simple_backup"] != "no" {
					t.Fatalf("trial payload %s", raw)
				}
				if _, ok := b["firewall_rules"]; ok {
					t.Fatal("trial firewall rules must stay immutable")
				}
				if deny {
					return 403, map[string]any{"error": map[string]any{"error_code": "TRIAL_FIREWALL", "error_message": "trial"}}, nil
				}
				return 202, map[string]any{"server": serverData{ID: serverID, State: "maintenance"}}, nil
			default:
				t.Fatalf("unexpected endpoint %s", r.URL.Path)
				return 500, nil, nil
			}
		}, nil)
		req := createRequest(t)
		req.UpCloudTrialCompatible = true
		result, err := d.CreateServer(context.Background(), req)
		if deny {
			if err == nil || result.Outcome != providers.OutcomeRejected || providers.IsRetryable(err) {
				t.Fatal(result, err)
			}
		} else if err != nil || result.ServerID != serverID {
			t.Fatal(result, err)
		}
		if posts != 1 {
			t.Fatal("create was retried", posts)
		}
	}
}
