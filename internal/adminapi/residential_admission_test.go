package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func admissionFixture(t *testing.T) (*tuningFixture, residentialperf.AdmissionEvidence, residentialperf.Request) {
	t.Helper()
	f := newTuningFixture(t, 2)
	ids := []string{perfUUID(), perfUUID(), perfUUID()}
	for _, id := range ids {
		sqlMust(t, f.db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,status,last_success_at,outbound_tag) VALUES($1::uuid,$1::text,'socks5','localhost',1080,'healthy',now(),'residential-ads-'||$1)", id)
	}
	if err := f.db.QueryRow("SELECT revision FROM residential_routing_control WHERE singleton").Scan(&f.rev); err != nil {
		t.Fatal(err)
	}
	f.verify(f.panels...)
	c, err := f.s.AdmissionContext(context.Background(), f.panels[0])
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	e := residentialperf.AdmissionEvidence{ID: perfUUID(), Manifest: residentialperf.AdmissionManifest, Context: c, Suspects: ids[:1], Controls: ids[1:], Started: now.Add(-time.Second), Finished: now, ContextStable: true, ChainVerified: true}
	for _, id := range ids {
		for _, target := range residentialperf.AdmissionTargets {
			for round := 0; round < 3; round++ {
				o := residentialperf.AdmissionObservation{ProxyID: id, Target: target.Name, Round: round, Started: e.Started, Finished: now, Outcome: "ok", HTTPStatus: target.Status, Milliseconds: 1000}
				if id == ids[0] && target.Name == "gpt" {
					o.Outcome = "timeout"
					o.CurlCode = 28
					o.HTTPStatus = 0
				}
				e.Observations = append(e.Observations, o)
			}
		}
	}
	e.CollectionHealthy = true
	e.CollectionOutcome = "completed"
	e.Guard = residentialperf.AdmissionGuard{Startup: "ready", NegativeCoreAlive: true, Positive: &residentialperf.AdmissionObservation{Outcome: "ok", HTTPStatus: 204}, Denied: &residentialperf.AdmissionObservation{Outcome: "transport", CurlCode: 56}}
	candidate := f.before.Clone()
	candidate.ExcludedProxyIDs = ids[:1]
	f.candidate = candidate
	q := f.startq()
	q.AdmissionEvidenceID = e.ID
	q.Minutes = 5
	return f, e, q
}
func TestAdmissionEvidenceEligibility(t *testing.T) {
	f, e, _ := admissionFixture(t)
	_ = f
	if err := e.Eligible(time.Now()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*residentialperf.AdmissionEvidence){
		"stale":          func(e *residentialperf.AdmissionEvidence) { e.Started = time.Now().Add(-6 * time.Minute) },
		"unproven chain": func(e *residentialperf.AdmissionEvidence) { e.ChainVerified = false },
		"drift":          func(e *residentialperf.AdmissionEvidence) { e.ContextStable = false },
		"partial":        func(e *residentialperf.AdmissionEvidence) { e.Observations = e.Observations[:35] },
		"duplicate":      func(e *residentialperf.AdmissionEvidence) { e.Observations[0] = e.Observations[1] },
		"failed control": func(e *residentialperf.AdmissionEvidence) { e.Observations[12].Outcome = "timeout" },
		"no repeated destination": func(e *residentialperf.AdmissionEvidence) {
			e.Observations[9].Outcome = "ok"
			e.Observations[9].HTTPStatus = 200
		},
		"transport failure": func(e *residentialperf.AdmissionEvidence) {
			for i := 9; i < 12; i++ {
				e.Observations[i].Outcome = "transport"
			}
		},
		"wrong success status": func(e *residentialperf.AdmissionEvidence) { e.Observations[12].HTTPStatus = 500 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(e)
			var x residentialperf.AdmissionEvidence
			json.Unmarshal(b, &x)
			change(&x)
			if x.Eligible(time.Now()) == nil {
				t.Fatal("unsafe evidence accepted")
			}
		})
	}
}
func TestAdmissionLifecycleIsolatedReplayExpiry(t *testing.T) {
	f, e, q := admissionFixture(t)
	ctx := context.Background()
	if err := f.s.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	beforeOther, _ := f.s.Load(ctx, f.panels[1])
	receipt, err := f.s.Do(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	f.phase("TESTING")
	candidate, _ := f.s.Load(ctx, f.panels[0])
	if !candidate.Config.Excludes(e.Suspects[0]) {
		t.Fatal("missing exclusion")
	}
	other, _ := f.s.Load(ctx, f.panels[1])
	if other.Generation != beforeOther.Generation {
		t.Fatal("unselected panel changed")
	}
	if _, err = f.s.Do(ctx, f.q("tune_publish")); err == nil {
		t.Fatal("admission published")
	}
	admissionVerify(t, f)
	f.tick()
	f.phase("TESTING")
	sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(now()-interval '1 minute')) WHERE id=$1", f.id)
	f.tick()
	f.phase("RESTORING")
	f.config(f.panels[0], f.before, candidate.Generation+1)
	replay, err := f.s.Do(ctx, q)
	if err != nil || replay != receipt {
		t.Fatal("committed replay failed", err)
	}
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
	q.RequestID = perfUUID()
	q.ExpectedVersion, _ = f.read()
	q.TuningID = ""
	if _, err = f.s.Do(ctx, q); err == nil {
		t.Fatal("evidence reused")
	}
}
func TestAdmissionRejectsMissingStaleWrongAndMixedEvidence(t *testing.T) {
	for _, which := range []string{"missing", "stale", "wrong-panel", "mixed", "two-panels", "control-failed", "context-drift", "permanent"} {
		t.Run(which, func(t *testing.T) {
			f, e, q := admissionFixture(t)
			ctx := context.Background()
			switch which {
			case "missing":
				q.AdmissionEvidenceID = ""
			case "stale":
				e.Started = time.Now().Add(-6 * time.Minute)
			case "wrong-panel":
				q.PanelIDs = f.panels[1:]
			case "mixed":
				q.Config.FastCount = 1
			case "two-panels":
				q.PanelIDs = f.panels
			case "control-failed":
				e.Observations[12].Outcome = "tls"
			case "context-drift":
				sqlMust(t, f.db, "UPDATE residential_proxies SET port=port+1 WHERE proxy_id=$1", e.Controls[0])
			case "permanent":
				q.Action = "start"
				q.Mode = "permanent"
				q.Scope = "fleet"
				q.PanelIDs = nil
				q.Minutes = 0
				q.AdmissionEvidenceID = ""
			}
			if err := f.s.RecordAdmission(ctx, e); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Do(ctx, q); err == nil {
				t.Fatal("unsafe admission accepted")
			}
			a, _ := f.s.Load(ctx, f.panels[0])
			if len(a.Config.ExcludedProxyIDs) > 0 {
				t.Fatal("failed request mutated assignment")
			}
		})
	}
}
func TestAdmissionContextDriftAndFailureRestore(t *testing.T) {
	for _, cause := range []string{"endpoint", "secret-version", "panel-identity", "route-revision", "native-failure", "ownership"} {
		t.Run(cause, func(t *testing.T) {
			f, e, q := admissionFixture(t)
			ctx := context.Background()
			if err := f.s.RecordAdmission(ctx, e); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Do(ctx, q); err != nil {
				t.Fatal(err)
			}
			a, _ := f.s.Load(ctx, f.panels[0])
			switch cause {
			case "endpoint":
				sqlMust(t, f.db, "UPDATE residential_proxies SET port=port+1 WHERE proxy_id=$1", e.Suspects[0])
			case "secret-version":
				sqlMust(t, f.db, "UPDATE residential_proxies SET admission_version=admission_version+1 WHERE proxy_id=$1", e.Controls[0])
			case "panel-identity":
				sqlMust(t, f.db, "UPDATE panel_instances SET base_url=base_url||'/changed' WHERE id=$1", f.panels[0])
			case "route-revision":
				sqlMust(t, f.db, "UPDATE residential_routing_control SET revision=revision+1")
			case "native-failure":
				f.s.Failed(ctx, f.panels[0], a.Generation, context.DeadlineExceeded)
			case "ownership":
				sqlMust(t, f.db, "UPDATE residential_performance_panels SET generation=generation+1 WHERE panel_id=$1", f.panels[0])
			}
			f.tick()
			f.phase("RESTORING")
			_, x := f.read()
			if cause == "ownership" {
				if x.Assigned || x.Conflict == "" {
					t.Fatal("ownership drift overwritten")
				}
			} else {
				f.config(f.panels[0], f.before, a.Generation+1)
			}
		})
	}
}
func TestAdmissionConcurrentSameRequestAndCallerOwnership(t *testing.T) {
	f, e, q := admissionFixture(t)
	ctx := context.Background()
	if err := f.s.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	q.Config.ExcludedProxyIDs[0] = strings.ToUpper(q.Config.ExcludedProxyIDs[0])
	original := q.Config.ExcludedProxyIDs[0]
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.Do(ctx, q); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if q.Config.ExcludedProxyIDs[0] != original {
		t.Fatal("mutated caller slice")
	}
	var n int
	if err := f.db.QueryRow("SELECT count(*) FROM residential_performance_operations WHERE admission_evidence_id=$1", e.ID).Scan(&n); err != nil || n != 1 {
		t.Fatal("receipt count", n, err)
	}
}

func TestAdmissionCredentialVersionImmutableReceiptAndDowngrade(t *testing.T) {
	f, e, q := admissionFixture(t)
	ctx := context.Background()
	id := e.Suspects[0]
	version := func() int64 {
		t.Helper()
		var v int64
		if err := f.db.QueryRow("SELECT admission_version FROM residential_proxies WHERE proxy_id=$1", id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	initial := version()
	sqlMust(t, f.db, "UPDATE residential_proxies SET last_checked_at=now(),status='down' WHERE proxy_id=$1", id)
	if version() != initial {
		t.Fatal("transient generic health invalidates identity")
	}
	sqlMust(t, f.db, "INSERT INTO residential_proxy_secrets(residential_id,id,kind,ciphertext,nonce,key_version) VALUES($1,'test','proxy_password','x','y',1)", id)
	if version() != initial+1 {
		t.Fatal("credential insertion did not advance identity")
	}
	sqlMust(t, f.db, "UPDATE residential_proxy_secrets SET ciphertext='z' WHERE residential_id=$1", id)
	if version() != initial+2 {
		t.Fatal("credential update did not advance identity")
	}
	sqlMust(t, f.db, "DELETE FROM residential_proxy_secrets WHERE residential_id=$1", id)
	if version() != initial+3 {
		t.Fatal("credential deletion did not advance identity")
	}
	if err := f.s.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE residential_admission_evidence SET evidence=evidence WHERE id=$1", e.ID); err == nil {
		t.Fatal("receipt was editable")
	}
	if _, err := f.s.Do(ctx, q); err == nil {
		t.Fatal("old credential evidence admitted")
	}
}
func TestAdmissionDowngradeGuard(t *testing.T) {
	f, e, q := admissionFixture(t)
	ctx := context.Background()
	if err := f.s.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Do(ctx, q); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../migrations/000166_residential_admission.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(string(b)); err == nil {
		tx.Rollback()
		t.Fatal("active admission downgrade allowed")
	}
	tx.Rollback()
	f.do("tune_cancel")
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
	tx, err = f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(string(b)); err != nil {
		t.Fatal("restored downgrade", err)
	}
}

func admissionVerify(t *testing.T, f *tuningFixture) {
	t.Helper()
	ctx := context.Background()
	a, err := f.s.Load(ctx, f.panels[0])
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = residentialperf.Lock(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.BindAdmissionPlanTx(ctx, tx, f.panels[0], a.Generation, "admission-plan"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE panel_routing_state SET plan_hash='admission-plan' WHERE panel_id=$1", f.panels[0]); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	f.verify(f.panels[0])
}
func TestAdmissionPlanOnlyDrift(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(fmt.Sprint(applied), func(t *testing.T) {
			f, e, q := admissionFixture(t)
			ctx := context.Background()
			if err := f.s.RecordAdmission(ctx, e); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Do(ctx, q); err != nil {
				t.Fatal(err)
			}
			if applied {
				admissionVerify(t, f)
			}
			sqlMust(t, f.db, "UPDATE panel_routing_state SET plan_hash='unrelated' WHERE panel_id=$1", f.panels[0])
			a, _ := f.s.Load(ctx, f.panels[0])
			if err := f.s.ValidateAdmissionAssignment(ctx, f.panels[0], a.Generation); err == nil {
				t.Fatal("unrelated plan admitted")
			}
			f.tick()
			f.phase("RESTORING")
			f.config(f.panels[0], f.before, a.Generation+1)
		})
	}
}

func TestAdmissionOneEvidenceTwoRequestRace(t *testing.T) {
	f, e, q := admissionFixture(t)
	ctx := context.Background()
	if err := f.s.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	other := q
	other.RequestID = perfUUID()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, request := range []residentialperf.Request{q, other} {
		request := request
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.Do(ctx, request); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("evidence admitted multiple operations", successes)
	}
}
func TestAdmissionPausedMutationCannotSupersedeCompletedRestore(t *testing.T) {
	for _, cause := range []string{"cancel", "expiry", "endpoint"} {
		t.Run(cause, func(t *testing.T) {
			f, e, q := admissionFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := f.s.RecordAdmission(ctx, e); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Do(ctx, q); err != nil {
				t.Fatal(err)
			}
			admissionVerify(t, f)
			a, _ := f.s.Load(ctx, f.panels[0])
			validated := make(chan struct{})
			resume := make(chan struct{})
			candidateDone := make(chan error, 1)
			restoreDone := make(chan error, 1)
			writes := make(chan string, 2)
			// This is the real cross-process PostgreSQL panel fence retained by
			// ReconcilePanel through applyAndVerify, native readback and generation ACK.
			go func() {
				candidateDone <- sanaei.WithConfigLock(ctx, f.db, f.panels[0], func(ctx context.Context) error {
					if err := f.s.ValidateAdmissionAssignment(ctx, f.panels[0], a.Generation); err != nil {
						return err
					}
					close(validated)
					select {
					case <-resume:
					case <-ctx.Done():
						return ctx.Err()
					}
					writes <- "candidate"
					if err := f.s.Applied(ctx, f.panels[0], a.Generation); err == nil {
						return fmt.Errorf("stale candidate generation acknowledged")
					}
					return nil
				})
			}()
			select {
			case <-validated:
			case err := <-candidateDone:
				t.Fatal(err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			switch cause {
			case "cancel":
				f.do("tune_cancel")
			case "expiry":
				sqlMust(t, f.db, "UPDATE residential_performance_experiments SET tuning=jsonb_set(tuning,'{deadline}',to_jsonb(now()-interval '1 minute')) WHERE id=$1", f.id)
				f.tick()
			case "endpoint":
				sqlMust(t, f.db, "UPDATE residential_proxies SET port=port+1 WHERE proxy_id=$1", e.Suspects[0])
				f.tick()
			}
			f.phase("RESTORING")
			go func() {
				restoreDone <- sanaei.WithConfigLock(ctx, f.db, f.panels[0], func(ctx context.Context) error { writes <- "before"; return nil })
			}()
			select {
			case err := <-restoreDone:
				t.Fatal("restore passed an in-flight native writer", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(resume)
			if err := <-candidateDone; err != nil {
				t.Fatal(err)
			}
			if err := <-restoreDone; err != nil {
				t.Fatal(err)
			}
			if <-writes != "candidate" || <-writes != "before" {
				t.Fatal("native writer ordering violated")
			}
			// A delayed old failure also cannot mark the new restoration generation rejected.
			f.s.Failed(ctx, f.panels[0], a.Generation, context.DeadlineExceeded)
			if cause == "endpoint" {
				f.db.QueryRow("SELECT revision FROM residential_routing_control WHERE singleton").Scan(&f.rev)
			}
			f.verify(f.panels[0])
			f.tick()
			f.phase("RESTORED")
		})
	}
}

func TestAdmissionSupportedUpgradeEntryGuard(t *testing.T) {
	f, e, q := admissionFixture(t)
	ctx := context.Background()
	if err := f.s.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Do(ctx, q); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := f.db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("BULK_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	params := u.Query()
	params.Del("search_path")
	params.Del("options")
	u.RawQuery = params.Encode()
	d := t.TempDir()
	env := filepath.Join(d, "env")
	manifest := filepath.Join(d, "manifest.json")
	os.WriteFile(env, []byte("DATABASE_URL=\""+u.String()+"\"\nMASTER_KEY_FILE=/tmp/unused\nMASTER_KEY_VERSION=1\nHTTP_ADDR=127.0.0.1:0\n"), 0600)
	bootstrap, _ := filepath.Abs("../../deploy/bootstrap.py")
	guard, _ := filepath.Abs("../../deploy/admission-compatibility.py")
	run := func(supported bool) error {
		t.Helper()
		body := "{}"
		if supported {
			body = `{"residential_admission_schema":1}`
		}
		os.WriteFile(manifest, []byte(body), 0600)
		cmd := exec.Command("python3", guard, bootstrap, env, manifest)
		cmd.Env = append(os.Environ(), "PGOPTIONS=-c search_path="+schema)
		return cmd.Run()
	}
	if run(false) == nil {
		t.Fatal("old consumer admitted during trial")
	}
	if err = run(true); err != nil {
		t.Fatal("capable consumer denied", err)
	}
	f.do("tune_cancel")
	if run(false) == nil {
		t.Fatal("old consumer admitted before restored proof")
	}
	f.verify(f.panels[0])
	f.tick()
	f.phase("RESTORED")
	if err = run(false); err != nil {
		t.Fatal("restored compatible downgrade blocked", err)
	}
	raw, _ := json.Marshal(f.candidate)
	sqlMust(t, f.db, "UPDATE residential_performance_panels SET config=$2 WHERE panel_id=$1", f.panels[0], string(raw))
	if run(false) == nil {
		t.Fatal("orphan exclusion admitted old reader")
	}
}
func TestAdmissionCredentialReassignmentVersionsBothEndpoints(t *testing.T) {
	f, e, _ := admissionFixture(t)
	first, second := e.Suspects[0], e.Controls[0]
	sqlMust(t, f.db, "INSERT INTO residential_proxy_secrets(residential_id,id,kind,ciphertext,nonce,key_version) VALUES($1,'test','proxy_password','x','y',1)", first)
	var beforeFirst, beforeSecond int64
	f.db.QueryRow("SELECT admission_version FROM residential_proxies WHERE proxy_id=$1", first).Scan(&beforeFirst)
	f.db.QueryRow("SELECT admission_version FROM residential_proxies WHERE proxy_id=$1", second).Scan(&beforeSecond)
	sqlMust(t, f.db, "UPDATE residential_proxy_secrets SET residential_id=$2 WHERE residential_id=$1", first, second)
	var afterFirst, afterSecond int64
	f.db.QueryRow("SELECT admission_version FROM residential_proxies WHERE proxy_id=$1", first).Scan(&afterFirst)
	f.db.QueryRow("SELECT admission_version FROM residential_proxies WHERE proxy_id=$1", second).Scan(&afterSecond)
	if afterFirst != beforeFirst+1 || afterSecond != beforeSecond+1 {
		t.Fatal("credential movement omitted an endpoint version")
	}
}

func TestAdmissionLatestReceiptAndCommittedReplay(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			f, e, q := admissionFixture(t)
			ctx := context.Background()
			if err := f.s.RecordAdmission(ctx, e); err != nil {
				t.Fatal(err)
			}
			var receipt residentialperf.Receipt
			if committed {
				var err error
				receipt, err = f.s.Do(ctx, q)
				if err != nil {
					t.Fatal(err)
				}
			}
			newer := e
			newer.ID = perfUUID()
			newer.ChainVerified = false
			// Receipt persistence must wait for the same lock held by authorization.
			tx, err := f.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err = residentialperf.Lock(ctx, tx); err != nil {
				t.Fatal(err)
			}
			recorded := make(chan error, 1)
			go func() { recorded <- f.s.RecordAdmission(ctx, newer) }()
			select {
			case err := <-recorded:
				t.Fatal("receipt bypassed authorization lock", err)
			case <-time.After(50 * time.Millisecond):
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-recorded; err != nil {
				t.Fatal(err)
			}
			actual, err := f.s.Do(ctx, q)
			if committed {
				if err != nil || actual != receipt {
					t.Fatal("newer evidence broke committed replay", err)
				}
			} else if err == nil {
				t.Fatal("superseded receipt authorized a new trial")
			}
		})
	}
}
func TestAdmissionLocalCollectorFailuresAreNotDestinationFailures(t *testing.T) {
	for _, which := range []string{"curl7", "legacy-connect", "core-exit"} {
		t.Run(which, func(t *testing.T) {
			f, e, q := admissionFixture(t)
			for i := 9; i < 12; i++ {
				switch which {
				case "curl7":
					e.Observations[i].Outcome = "local_proxy_unavailable"
					e.Observations[i].CurlCode = 7
				case "legacy-connect":
					e.Observations[i].Outcome = "connect"
					e.Observations[i].CurlCode = 7
				}
			}
			if which == "core-exit" {
				e.CollectionHealthy = false
				e.CollectionOutcome = "diagnostic_core_exited"
			}
			if err := f.s.RecordAdmission(context.Background(), e); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.Do(context.Background(), q); err == nil {
				t.Fatal("local collector failure authorized admission")
			}
		})
	}
}
