package transportapply

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type memorySecrets struct{ values map[string][]byte }

func (m *memorySecrets) Get(_ context.Context, _, ref string) ([]byte, error) {
	v, ok := m.values[ref]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte{}, v...), nil
}
func (m *memorySecrets) Put(_ context.Context, _, ref, _ string, v []byte) error {
	m.values[ref] = append([]byte{}, v...)
	return nil
}

type fakeSSH struct {
	verifyFail    bool
	preflights    int
	verifications int
}

func (s *fakeSSH) Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error {
	return nil
}
func (s *fakeSSH) Run(_ context.Context, _ provisioning.Target, _ []byte, cmd string) (string, error) {
	if strings.Contains(cmd, "DOB_RUNTIME_PROCESS") {
		return "{\"process\":\"fixture-old-process\"}", nil
	}
	if strings.Contains(cmd, "DOB_VERIFY_TRANSPORT") {
		s.verifications++
		if s.verifyFail {
			return "", errors.New("runtime not ready")
		}
		return "{\"verified\":true}", nil
	}
	if strings.Contains(cmd, "DOB_TRANSPORT_RUNTIME") {
		s.preflights++
		return "{\"validated\":true}", nil
	}
	return "", nil
}
func TestDurableApplyRecoversUnknownWriteThenRollsBack(t *testing.T) { runDurableApply(t, false) }
func TestBlockedNativeWriteCanRestoreOriginalContract(t *testing.T)  { runDurableApply(t, true) }
func runDurableApply(t *testing.T, blockedRecovery bool) {
	db := applyTestDB(t)
	ctx := context.Background()
	before, old := fixture(t)
	var mu sync.Mutex
	native := append([]byte{}, before...)
	writes := 0
	restarts := 0
	unreadable := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/csrf-token") {
			io.WriteString(w, `{"success":true,"obj":"csrf"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/login") {
			io.WriteString(w, "{\"success\":true}")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/restartXrayService") {
			restarts++
			io.WriteString(w, `{"success":true}`)
			return
		}
		if strings.Contains(r.URL.Path, "/update/") {
			body, _ := io.ReadAll(r.Body)
			native = body
			writes++
			unreadable = true
			w.WriteHeader(503)
			io.WriteString(w, "{\"success\":false}")
			return
		}
		if unreadable {
			w.WriteHeader(503)
			io.WriteString(w, "{\"success\":false}")
			return
		}
		io.WriteString(w, "{\"success\":true,\"obj\":["+string(native)+"]}")
	}))
	defer ts.Close()
	account, droplet, panel := seedPanel(t, db, ts.URL)
	sqlMust(t, db, "UPDATE deployments SET host='127.0.0.1',profile_snapshot='{\"ssh_user\":\"root\",\"ssh_key_secret_ref\":\"ssh\"}' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	sqlMust(t, db, "INSERT INTO panel_inbound_inventory(panel_id,remote_id,remark,protocol,transport,security,port,present,enabled,raw_hash,first_seen_at,last_seen_at) VALUES($1,42,$2,'vless','xhttp','reality',443,true,true,'fixture',now(),now())", panel, testKey)
	raw, _ := json.Marshal(old)
	sqlMust(t, db, "INSERT INTO reality_listener_profiles(panel_id,managed_key,config,path,policy_revision) VALUES($1,$2,$3,$4,1)", panel, testKey, raw, old.Path)
	sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,uri,client_id) VALUES($1,'vless://old','kept')", panel)
	desired := transportprofile.Recommended()
	desired.Host = "aparat.com"
	policy := transportprofile.Policy{Revision: 2, Config: desired}
	tx, _ := db.BeginTx(ctx, nil)
	q, e := Queue(ctx, tx, policy, panel)
	if e != nil || q.Queued != 1 {
		t.Fatal(q, e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	var id string
	if e = db.QueryRow("SELECT id::text FROM reality_transport_apply_jobs").Scan(&id); e != nil {
		t.Fatal(e)
	}
	// Persisted admission guard covers every mutation producer, including lifecycle.
	_, e = db.Exec("INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,42,'new','CREATE','during-apply','{}')", account, panel)
	if e == nil {
		t.Fatal("client reservation raced transport")
	}
	tx, _ = db.BeginTx(ctx, nil)
	_, e = Queue(ctx, tx, policy)
	tx.Rollback()
	if !errors.Is(e, ErrBusy) {
		t.Fatal("overlap accepted", e)
	}
	secrets := &memorySecrets{map[string][]byte{"test": []byte("test"), "ssh": []byte("test-key")}}
	ssh := &fakeSSH{}
	service := Service{DB: db, Secrets: secrets, SSH: ssh, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets}}}
	if e = service.RunOne(ctx); e == nil {
		t.Fatal("unknown result published")
	}
	var state, phase string
	db.QueryRow("SELECT state,phase FROM reality_transport_apply_jobs WHERE id=$1", id).Scan(&state, &phase)
	if state != "RUNNING" || phase != "WRITE_SENT" {
		t.Fatal(state, phase, e)
	}
	sqlMust(t, db, "UPDATE droplets SET state='RETIRING' WHERE id=$1", droplet)
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET next_retry_at=now() WHERE id=$1", id)
	if service.RunOne(ctx) == nil {
		t.Fatal("unresolved write retired without fence")
	}
	db.QueryRow("SELECT state FROM reality_transport_apply_jobs WHERE id=$1", id).Scan(&state)
	if state != "BLOCKED" {
		t.Fatal("unresolved native outcome released", state)
	}
	sqlMust(t, db, "UPDATE droplets SET state='READY' WHERE id=$1", droplet)
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET state='RUNNING',phase='RECHECK_REQUESTED',attempts=0,next_retry_at=now() WHERE id=$1", id)
	db.QueryRow("SELECT config FROM reality_listener_profiles WHERE panel_id=$1", panel).Scan(&raw)
	var unchanged transportprofile.Config
	json.Unmarshal(raw, &unchanged)
	if !jsonEqual(old, unchanged) {
		t.Fatal("pin promoted before verification")
	}
	mu.Lock()
	unreadable = false
	mu.Unlock()
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET next_retry_at=now() WHERE id=$1", id)
	// Simulate process restart: fresh manager, durable encrypted before state.
	service.Runtimes = &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets}}
	ssh.verifyFail = true
	if service.RunOne(ctx) == nil {
		t.Fatal("unverified runtime promoted")
	}
	mu.Lock()
	count := writes
	mu.Unlock()
	if count != 1 {
		t.Fatal("blind POST retry", count)
	}
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET next_retry_at=now(),phase='RECHECK_REQUESTED' WHERE id=$1", id)
	if service.RunOne(ctx) == nil {
		t.Fatal("unverified restart accepted")
	}
	mu.Lock()
	restartCount := restarts
	mu.Unlock()
	if restartCount != 1 {
		t.Fatal("restart replay", restartCount)
	}
	if blockedRecovery {
		sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET state='BLOCKED' WHERE id=$1", id)
		tx, _ = db.BeginTx(ctx, nil)
		if e = Rollback(ctx, tx, id); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
		var active int
		db.QueryRow("SELECT count(*) FROM reality_transport_apply_jobs WHERE state='RUNNING'").Scan(&active)
		if active != 1 {
			t.Fatal("rollback publication fence absent")
		}
		ssh.verifyFail = false
		if service.RunOne(ctx) == nil {
			t.Fatal("expected restore lost acknowledgement")
		}
		mu.Lock()
		unreadable = false
		mu.Unlock()
		sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET next_retry_at=now() WHERE state='RUNNING'")
		if e = service.RunOne(ctx); e != nil {
			t.Fatal("blocked recovery", e)
		}
		db.QueryRow("SELECT config FROM reality_listener_profiles WHERE panel_id=$1", panel).Scan(&raw)
		var restored transportprofile.Config
		json.Unmarshal(raw, &restored)
		if !jsonEqual(old, restored) {
			t.Fatal("original pin lost")
		}
		mu.Lock()
		defer mu.Unlock()
		if writes != 2 || Verify(before, native, testKey, old) != nil {
			t.Fatal("blocked native restore mismatch")
		}
		return
	}
	ssh.verifyFail = false
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET next_retry_at=now() WHERE id=$1", id)
	if e = service.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	db.QueryRow("SELECT state FROM reality_transport_apply_jobs WHERE id=$1", id).Scan(&state)
	if state != "SUCCEEDED" {
		t.Fatal(state)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM output_config_snapshots WHERE panel_id=$1", panel).Scan(&n)
	if n != 0 {
		t.Fatal("old links retained")
	}
	db.QueryRow("SELECT config FROM reality_listener_profiles WHERE panel_id=$1", panel).Scan(&raw)
	var current transportprofile.Config
	json.Unmarshal(raw, &current)
	if current.Host != "aparat.com" || !current.MaskOnClient() || current.Flow != transportprofile.VisionUDP443 {
		t.Fatal("new export contract absent")
	}
	tx, _ = db.BeginTx(ctx, nil)
	if e = Rollback(ctx, tx, id); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	tx, _ = db.BeginTx(ctx, nil)
	e = Rollback(ctx, tx, id)
	tx.Rollback()
	if !errors.Is(e, ErrBusy) {
		t.Fatal("overlapping rollback", e)
	}
	// Native write again loses its acknowledgement; recovery still does one write.
	if service.RunOne(ctx) == nil {
		t.Fatal("expected injected read failure")
	}
	mu.Lock()
	unreadable = false
	mu.Unlock()
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET next_retry_at=now() WHERE state='RUNNING'")
	if e = service.RunOne(ctx); e != nil {
		t.Fatal("rollback", e)
	}
	db.QueryRow("SELECT config FROM reality_listener_profiles WHERE panel_id=$1", panel).Scan(&raw)
	json.Unmarshal(raw, &current)
	// Unmarshal into a zero struct, because omitted fields must reset.
	current = transportprofile.Config{}
	json.Unmarshal(raw, &current)
	if !jsonEqual(old, current) {
		t.Fatal("rollback profile not restored")
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 2 || Verify(before, native, testKey, old) != nil {
		t.Fatal("rollback native/clients")
	}
}

var _ *sql.DB

func TestUndrainableClientWorkReleasesPrewriteAdmission(t *testing.T) {
	db := applyTestDB(t)
	ctx := context.Background()
	account, _, panel := seedPanel(t, db, "http://fixture")
	sqlMust(t, db, "INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,1,'pending','CREATE','before-apply','{}')", account, panel)
	old := transportprofile.Config{Network: "tcp"}
	target, e := Target(old, transportprofile.Recommended())
	if e != nil {
		t.Fatal(e)
	}
	tx, _ := db.BeginTx(ctx, nil)
	if e = enqueue(ctx, tx, panel, testKey, 1, old, target, nil); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "UPDATE reality_transport_apply_jobs SET created_at=now()-interval '3 minutes'")
	if e = (Service{DB: db}).RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	var state string
	db.QueryRow("SELECT state FROM reality_transport_apply_jobs").Scan(&state)
	if state != "FAILED" {
		t.Fatal("prewrite admission retained", state)
	}
	db.QueryRow("SELECT state FROM client_mutation_jobs").Scan(&state)
	if state != "PENDING" {
		t.Fatal("client intent lost", state)
	}
	sqlMust(t, db, "INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,1,'new','CREATE','after-release','{}')", account, panel)
}
