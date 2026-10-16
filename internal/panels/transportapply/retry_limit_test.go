package transportapply

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRetryLimitOnlyBlocksUnresolvedNativeChanges(t *testing.T) {
	for _, tc := range []struct {
		name, ref, phase string
		remote           any
		restart          bool
		rollback         bool
		rollbackJournal  bool
		unknownField     bool
		want             string
	}{
		{name: "unstarted_read_failure", phase: "QUEUED", want: "FAILED"},
		{name: "unstarted_manual_recheck", phase: "RECHECK_REQUESTED", want: "FAILED"},
		{name: "unknown_saved_field", phase: "QUEUED", unknownField: true, want: "BLOCKED"},
		{name: "saved_backup", phase: "PREPARING", ref: "backup", remote: int64(42), want: "BLOCKED"},
		{name: "unknown_write", phase: "WRITE_SENT", ref: "backup", remote: int64(42), want: "BLOCKED"},
		{name: "rollback_journal", phase: "QUEUED", rollbackJournal: true, want: "BLOCKED"},
		{name: "rollback_source", phase: "QUEUED", rollback: true, want: "BLOCKED"},
		{name: "inconsistent_remote", phase: "QUEUED", remote: int64(42), want: "BLOCKED"},
		{name: "inconsistent_restart", phase: "QUEUED", restart: true, want: "BLOCKED"},
		{name: "inconsistent_write_phase", phase: "WRITE_SENT", want: "BLOCKED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := applyTestDB(t)
			account, _, panel := seedPanel(t, db, "http://127.0.0.1:1")
			_, before := fixture(t)
			target := before
			target.Host = "changed.example"
			source := before
			if tc.rollback {
				source = target
			}
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(target)
			c, _ := json.Marshal(source)
			if tc.unknownField {
				var v map[string]any
				json.Unmarshal(c, &v)
				v["future_setting"] = true
				c, _ = json.Marshal(v)
			}
			sqlMust(t, db, `INSERT INTO reality_transport_apply_jobs(id,panel_id,managed_key,policy_revision,before_config,target_config,source_config,before_secret_ref,remote_id,state,phase,attempts,restart_sent_at,rollback_of) VALUES('00000000-0000-4000-8000-000000000001',$1,$2,1,$3,$4,$5,$6,$7,'RUNNING',$8,5,CASE WHEN $9 THEN now() ELSE NULL END,CASE WHEN $10 THEN '00000000-0000-4000-8000-000000000001'::uuid ELSE NULL END)`, panel, testKey, a, b, c, tc.ref, tc.remote, tc.phase, tc.restart, tc.rollbackJournal)
			err := (Service{DB: db}).RunOne(context.Background())
			if (err == nil) != (tc.want == "FAILED") {
				t.Fatalf("unexpected retry-limit error: %v", err)
			}
			var state string
			if e := db.QueryRow("SELECT state FROM reality_transport_apply_jobs").Scan(&state); e != nil {
				t.Fatal(e)
			}
			if state != tc.want {
				t.Fatalf("retry exhaustion state=%s, want %s", state, tc.want)
			}
			_, e := db.Exec("INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,42,'client','CREATE','after-exhaustion','{}')", account, panel)
			if (e == nil) != (tc.want == "FAILED") {
				t.Fatalf("native write admission mismatch: state=%s err=%v", state, e)
			}
		})
	}
}
