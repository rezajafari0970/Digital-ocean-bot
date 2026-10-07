package observability

import (
	"encoding/json"
	"testing"
	"time"
)

func TestClientProgressReadinessSeparatesIdleGateFailureAndStall(t *testing.T) {
	now := time.Now()
	fresh := now.Unix()
	old := now.Add(-3 * time.Minute).Unix()
	cases := []struct {
		state             string
		started, finished int64
		want              bool
	}{
		{"", 0, 0, false}, {"IDLE", old, fresh, true}, {"GATED", old, fresh, true},
		{"COMPLETED", old, fresh, true}, {"FAILED", fresh, fresh, false},
		{"RUNNING", fresh, old, true}, {"RUNNING", old, fresh, false},
		{"IDLE", fresh, old, false}, {"RUNNING", now.Add(time.Hour).Unix(), fresh, false},
	}
	for _, tt := range cases {
		raw, _ := json.Marshal(map[string]any{"client_mutation": map[string]any{"state": tt.state, "started_unix": tt.started, "finished_unix": tt.finished}})
		if got := clientProgressReady(raw, now); got != tt.want {
			t.Fatalf("%+v got %v", tt, got)
		}
	}
	for _, raw := range []string{"{}", "null", "{", `{"client_mutation":{"state":"IDLE","finished_unix":"bad"}}`} {
		if clientProgressReady([]byte(raw), now) {
			t.Fatal("malformed progress ready", raw)
		}
	}
}

func TestLifecycleLaneReadinessDoesNotHideUncooperativeHandler(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want bool
	}{
		{`{"lifecycle_lanes":{"in_flight":0,"stalled":0}}`, true},
		{`{"lifecycle_lanes":{"in_flight":3,"stalled":0}}`, true},
		{`{"lifecycle_lanes":{"in_flight":3,"stalled":1}}`, false},
		{"{}", false}, {"null", false}, {"{", false},
	} {
		if got := lifecycleLanesReady([]byte(tt.raw)); got != tt.want {
			t.Fatal(tt, got)
		}
	}
}
