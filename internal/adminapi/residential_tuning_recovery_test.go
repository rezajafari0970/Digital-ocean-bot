package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTuningFleetPreflightRejectsUnverifiedSurvivorAtomically(t *testing.T) {
	cases := map[string]string{
		"retiring_invalid_account": `UPDATE accounts SET enabled=false,provider_state='TOKEN_INVALID',deletion_requested_at=now() WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1);
 UPDATE droplets SET state='RETIRING' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1);
 UPDATE panel_routing_state SET state='FAILED' WHERE panel_id=$1`,
		"future_profile":    `UPDATE residential_performance_panels SET verified_at=now()+interval '1 hour' WHERE panel_id=$1`,
		"future_router":     `UPDATE panel_routing_state SET verified_at=now()+interval '1 hour' WHERE panel_id=$1`,
		"stale_profile":     `UPDATE residential_performance_panels SET verified_at=now()-interval '61 seconds' WHERE panel_id=$1`,
		"stale_router":      `UPDATE panel_routing_state SET verified_at=now()-interval '61 seconds' WHERE panel_id=$1`,
		"router_generation": `UPDATE panel_routing_state SET performance_generation=0 WHERE panel_id=$1`,
		"unapplied_profile": `UPDATE residential_performance_panels SET applied_generation=0 WHERE panel_id=$1`,
		"router_applying":   `UPDATE panel_routing_state SET state='APPLYING' WHERE panel_id=$1`,
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			f := newTuningFixture(t, 2)
			f.start()
			f.verify(f.panels[0])
			for _, query := range strings.Split(sql, ";") {
				if strings.TrimSpace(query) != "" {
					sqlMust(t, f.db, query, f.panels[1])
				}
			}
			version, prior := f.read()
			var operations int
			f.db.QueryRow("SELECT count(*) FROM residential_performance_operations").Scan(&operations)
			if _, err := f.s.Do(context.Background(), f.q("tune_publish")); err == nil || !strings.Contains(err.Error(), "fresh runtime proof for every surviving assignment") {
				t.Fatal("known-unready fleet accepted", err)
			}
			current, after := f.read()
			a, _ := json.Marshal(prior)
			b, _ := json.Marshal(after)
			if version != current || string(a) != string(b) {
				t.Fatal("rejected publication changed tuning")
			}
			f.config(f.panels[0], f.candidate, 2)
			f.config(f.panels[1], f.before, 1)
			var nowOps int
			f.db.QueryRow("SELECT count(*) FROM residential_performance_operations").Scan(&nowOps)
			if nowOps != operations {
				t.Fatal("rejected action created success receipt")
			}
			var spec []byte
			f.db.QueryRow("SELECT spec FROM residential_performance_experiments WHERE id=$1", f.id).Scan(&spec)
			var count, share int
			var v map[string]any
			json.Unmarshal(spec, &v)
			count = int(v["fast_count"].(float64))
			share = int(v["fast_share"].(float64))
			if count != 13 || share != 80 {
				t.Fatal("rejected publication changed fleet defaults")
			}
			// A real fresh proof can admit the same fleet without deleting its obligations.
			f.verify(f.panels...)
			f.do("tune_publish")
			f.phase("PUBLISHING")
		})
	}
}

func TestTuningFailureMetadataIsFirstTriggerOnlyAndSanitized(t *testing.T) {
	f := newTuningFixture(t, 2)
	f.start()
	ctx := context.Background()
	f.s.FailedAt(ctx, f.panels[0], 1, errors.New("stale result"), "panel_snapshot")
	f.phase("TESTING")
	f.s.FailedAt(ctx, f.panels[0], 2, errors.New("https://secret:password@example.invalid/?token=secret"), "panel_snapshot")
	_, first := f.read()
	if first.Failure == nil || first.Failure.PanelID != f.panels[0] || first.Failure.Generation != 2 || first.Failure.Stage != "panel_snapshot" || first.Failure.At.IsZero() || first.Phase != "RESTORING" {
		t.Fatal("missing first failure metadata", first)
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), "password") || strings.Contains(string(raw), "token=secret") {
		t.Fatal("raw error leaked")
	}
	f.s.FailedAt(ctx, f.panels[0], 2, errors.New("later"), "settings_read")
	_, after := f.read()
	a, _ := json.Marshal(first.Failure)
	b, _ := json.Marshal(after.Failure)
	if string(a) != string(b) {
		t.Fatal("later error overwrote first trigger")
	}
	f.tick()
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
	_, after = f.read()
	if after.Failure == nil || !after.Rejected {
		t.Fatal("recovery erased failure history")
	}
	f.start()
	f.s.FailedAt(ctx, f.panels[0], 4, errors.New("opaque"), "https://secret.invalid")
	_, after = f.read()
	if after.Failure == nil || after.Failure.Stage != "unspecified" {
		t.Fatal("untrusted failure stage retained")
	}
}

func TestTuningProgressShowsBlockedSurvivorWithoutCompletingRecovery(t *testing.T) {
	f := newTuningFixture(t, 2)
	f.start()
	f.verify(f.panels[0])
	f.do("tune_publish")
	f.s.FailedAt(context.Background(), f.panels[1], 2, errors.New("timeout"), "panel_snapshot")
	f.tick()
	f.verify(f.panels[0])
	sqlMust(t, f.db, "UPDATE accounts SET enabled=false,provider_state='TOKEN_INVALID',deletion_requested_at=now() WHERE id=(SELECT account_id FROM panel_instances WHERE id=$1)", f.panels[1])
	sqlMust(t, f.db, "UPDATE droplets SET state='RETIRING' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)", f.panels[1])
	f.tick()
	f.phase("RESTORING")
	r := httptest.NewRequest("GET", "/api/v1/residential-performance", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	response := httptest.NewRecorder()
	(&Server{DB: f.db}).residentialPerformanceStatus(response, r)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var body struct {
		Experiment struct {
			Tuning   struct{ Phase string } `json:"active_tuning"`
			Progress struct {
				Surviving, Verified, Pending int
				NonServing                   int `json:"non_serving"`
				Credentials                  int `json:"credential_blocked"`
			} `json:"tuning_progress"`
		} `json:"experiment"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	p := body.Experiment.Progress
	if body.Experiment.Tuning.Phase != "RESTORING" || p.Surviving != 2 || p.Verified != 1 || p.Pending != 1 || p.NonServing != 1 || p.Credentials != 1 {
		t.Fatal("misleading recovery progress", response.Body.String())
	}
	f.phase("RESTORING")
}

func TestPerformanceStatusCannotCallStaleRoutingVerified(t *testing.T) {
	f := newTuningFixture(t, 1)
	f.start()
	f.verify(f.panels[0])
	sqlMust(t, f.db, "UPDATE panel_routing_state SET verified_at=now()-interval '61 seconds' WHERE panel_id=$1", f.panels[0])
	request := httptest.NewRequest("GET", "/api/v1/residential-performance", nil)
	request = request.WithContext(context.WithValue(request.Context(), principalKey{}, auth.Principal{Role: auth.Admin}))
	response := httptest.NewRecorder()
	(&Server{DB: f.db}).residentialPerformanceStatus(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var body struct {
		Experiment struct {
			Counts  struct{ Verified int }
			Targets []struct {
				Verified bool
				Reason   string `json:"recovery_reason"`
			}
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Experiment.Counts.Verified != 0 || len(body.Experiment.Targets) != 1 || body.Experiment.Targets[0].Verified || body.Experiment.Targets[0].Reason != "PROOF_STALE" {
		t.Fatal(response.Body.String())
	}
}
