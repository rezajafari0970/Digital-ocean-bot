package adminapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRequestCommittedLegacyReplay(t *testing.T) {
	f := newTuningFixture(t, 1)
	raw, err := os.ReadFile("../residentialperf/testdata/request-legacy-fc76e6a.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Request residentialperf.Request `json:"request"`
		Hash    string                  `json:"hash"`
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		identity := strings.ToLower(c.Request.ExperimentID)
		if identity == "" {
			identity = strings.ToLower(c.Request.RequestID)
		}
		expected := residentialperf.Receipt{ExperimentID: identity, State: "KEPT", Version: 123}
		response, _ := json.Marshal(expected)
		sqlMust(t, f.db, "INSERT INTO residential_performance_operations(request_id,request_hash,response) VALUES($1,$2,$3)", c.Request.RequestID, c.Hash, string(response))
		got, err := f.s.Do(context.Background(), c.Request)
		if err != nil || got != expected {
			t.Fatal("deployed legacy operation did not replay", c.Request.Action, err)
		}
		reconciled, err := f.s.Reconcile(context.Background(), c.Request)
		if err != nil || reconciled == nil || *reconciled != expected {
			t.Fatal("legacy read-only reconciliation failed", err)
		}
	}
}

func TestStabilityActualCLIUnknownResponseAndWorkerRecovery(t *testing.T) {
	binary := os.Getenv("STABILITY_TEST_TUNE_BINARY")
	if binary == "" {
		t.Skip("explicit reviewed candidate CLI required")
	}
	for _, cause := range []string{"cancel", "stale-cancel-deadline"} {
		t.Run(cause, func(t *testing.T) {
			f, second, q := admissionFixture(t)
			ctx := context.Background()
			first := earlierStability(second, 2*time.Minute)
			q.StabilityEvidenceID = first.ID
			for _, e := range []residentialperf.AdmissionEvidence{first, second} {
				if err := f.s.RecordAdmission(ctx, e); err != nil {
					t.Fatal(err)
				}
			}
			var schema string
			if err := f.db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
				t.Fatal(err)
			}
			dsn, err := url.Parse(os.Getenv("BULK_TEST_DATABASE_URL"))
			if err != nil {
				t.Fatal(err)
			}
			params := dsn.Query()
			params.Set("search_path", schema)
			dsn.RawQuery = params.Encode()
			env := []string{"PATH=" + os.Getenv("PATH"), "DATABASE_URL=" + dsn.String(), "MASTER_KEY_B64=" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), "MASTER_KEY_VERSION=1"}
			invoke := func(action string, request residentialperf.Request, loseResponse bool) ([]byte, error) {
				t.Helper()
				raw, _ := json.Marshal(request)
				cmd := exec.Command(binary, action)
				cmd.Env = env
				cmd.Stdin = bytes.NewReader(raw)
				if loseResponse {
					cmd.Stdout = io.Discard
					cmd.Stderr = io.Discard
					return nil, cmd.Run()
				}
				return cmd.Output()
			}
			// Execute the actual operator CLI and deliberately discard its committed reply.
			if _, err = invoke("--execute", q, true); err != nil {
				t.Fatal("isolated CLI start failed")
			}
			data, err := invoke("--reconcile", q, false)
			if err != nil {
				t.Fatal("exact CLI reconciliation failed")
			}
			var result struct {
				Found    bool `json:"found"`
				Verified bool `json:"exact_request_verified"`
			}
			if err = json.Unmarshal(data, &result); err != nil || !result.Found || !result.Verified {
				t.Fatal("lost reply unconfirmed", err)
			}
			changed := q
			changed.Minutes = 6
			if _, err = invoke("--reconcile", changed, false); err == nil {
				t.Fatal("CLI reconciled different full request")
			}
			if _, err = invoke("--execute", q, true); err != nil {
				t.Fatal("exact replay failed")
			}
			admissionVerify(t, f)
			cancel := f.q("tune_cancel")
			if cause == "stale-cancel-deadline" {
				// Binding saves a new owned version; the original cancellation remains immutable.
				sqlMust(t, f.db, "UPDATE residential_performance_experiments SET version=version+1 WHERE id=$1", f.id)
				if _, err = invoke("--execute", cancel, true); err == nil {
					t.Fatal("stale cancellation succeeded")
				}
				f.phase("TESTING")
				sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(clock_timestamp()-interval '1 second')) WHERE id=$1", f.id)
				f.tick()
			} else {
				if _, err = invoke("--execute", cancel, true); err != nil {
					t.Fatal("owned CLI cancellation failed")
				}
			}
			f.phase("RESTORING")
			f.verify(f.panels[0])
			f.tick()
			f.phase("RESTORED")
			assignment, err := f.s.Load(ctx, f.panels[0])
			if err != nil || len(assignment.Config.ExcludedProxyIDs) != 0 || assignment.Config.FastCount != 13 {
				t.Fatal("worker did not restore exact parent", err)
			}
			var count int
			if err = f.db.QueryRow("SELECT count(*) FROM residential_performance_operations WHERE admission_evidence_id IS NOT NULL").Scan(&count); err != nil || count != 1 {
				t.Fatal("duplicate admission", count, err)
			}
		})
	}
}
