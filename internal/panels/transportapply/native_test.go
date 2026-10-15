package transportapply

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type fixtureSSH struct{}

func (fixtureSSH) Upload(ctx context.Context, _ provisioning.Target, _ []byte, local, remote string, mode os.FileMode) error {
	if !strings.HasPrefix(remote, "/root/.dob-transport-check-") {
		return fmt.Errorf("fixture path")
	}
	if e := exec.CommandContext(ctx, "lxc", "file", "push", local, "dobapply24"+remote).Run(); e != nil {
		return e
	}
	return exec.CommandContext(ctx, "lxc", "exec", "dobapply24", "--", "chmod", fmt.Sprintf("%o", mode), remote).Run()
}
func (fixtureSSH) Run(ctx context.Context, _ provisioning.Target, _ []byte, cmd string) (string, error) {
	out, e := exec.CommandContext(ctx, "lxc", "exec", "dobapply24", "--", "sh", "-c", cmd).CombinedOutput()
	if strings.Contains(cmd, "DOB_VERIFY_TRANSPORT") {
		fmt.Printf("native verifier: %s", out)
		diag, _ := exec.CommandContext(ctx, "lxc", "exec", "dobapply24", "--", "python3", "-c", `import json;print([(i.get("port"),i.get("protocol")) for i in json.load(open("/usr/local/x-ui/bin/config.json")).get("inbounds",[])])`).CombinedOutput()
		fmt.Printf("native ports: %s", diag)
	}
	return string(out), e
}
func TestNativeApplyFixture(t *testing.T) {
	base := os.Getenv("DOB_NATIVE_APPLY_URL")
	if base == "" {
		t.Skip("explicit disposable native fixture required")
	}
	if base != "http://10.231.74.20:2053/fixture" {
		t.Fatal("native fixture scope refused")
	}
	db := applyTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	_, droplet, panel := seedPanel(t, db, base)
	sqlMust(t, db, "UPDATE deployments SET host='10.231.74.20',profile_snapshot='{\"ssh_user\":\"root\",\"ssh_key_secret_ref\":\"ssh\"}' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE xui_panel_deployments SET username='fixture' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,users_per_second) VALUES('reality',true,'[443]',2,1) ON CONFLICT(policy_key) DO UPDATE SET enabled=true")
	secrets := &memorySecrets{map[string][]byte{"test": []byte("fixture-test-password"), "ssh": []byte("fixture-key")}}
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: 10 * time.Second}}
	rt, e := manager.Acquire(ctx, panel)
	if e != nil {
		t.Fatal("native login", e)
	}
	before, old := fixture(t)
	_, payload, e := Transform(before, testKey, old)
	if e != nil {
		t.Fatal(e)
	}
	payload.Port = 38447
	for _, v := range payload.Settings["clients"].([]any) {
		c := v.(map[string]any)
		id, _ := sanaei.UUIDv4()
		c["id"] = id
		c["email"] = "apply-fixture-" + id
	}
	if _, e = sanaei.AddInbound(ctx, rt.Session.Exec, payload); e != nil {
		t.Fatal("native add", e)
	}
	actual, id, e := read(ctx, rt, testKey)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		if e := sanaei.DeleteInbound(cleanup, rt.Session.Exec, id); e != nil {
			t.Error("fixture cleanup", e)
		}
	}()
	if e = Verify(actual, actual, testKey, old); e != nil {
		t.Fatal("native baseline", e)
	}
	sqlMust(t, db, "INSERT INTO panel_inbound_inventory(panel_id,remote_id,remark,protocol,transport,security,port,present,enabled,raw_hash,first_seen_at,last_seen_at) VALUES($1,$2,$3,'vless','xhttp','reality',38447,true,true,'fixture',now(),now())", panel, id, testKey)
	raw, _ := json.Marshal(old)
	sqlMust(t, db, "INSERT INTO reality_listener_profiles(panel_id,managed_key,config,path,policy_revision) VALUES($1,$2,$3,$4,1)", panel, testKey, raw, old.Path)
	desired := transportprofile.Recommended()
	desired.Host = "aparat.com"
	tx, _ := db.BeginTx(ctx, nil)
	_, e = Queue(ctx, tx, transportprofile.Policy{Revision: 2, Config: desired}, panel)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	service := Service{DB: db, Secrets: secrets, Runtimes: manager, SSH: fixtureSSH{}}
	if e = service.RunOne(ctx); e != nil {
		var phase, code string
		db.QueryRow("SELECT phase,last_error FROM reality_transport_apply_jobs").Scan(&phase, &code)
		t.Fatalf("native apply: %v phase=%s code=%s", e, phase, code)
	}
	var state, jobID string
	db.QueryRow("SELECT state,id::text FROM reality_transport_apply_jobs").Scan(&state, &jobID)
	if state != "SUCCEEDED" {
		t.Fatal("native apply state", state)
	}
	after, _, e := read(ctx, rt, testKey)
	if e != nil {
		t.Fatal(e)
	}
	target, _ := Target(old, desired)
	if e = Verify(actual, after, testKey, target); e != nil {
		t.Fatal("native client preservation", e)
	}
	_, verifiedPayload, e := Transform(after, testKey, target)
	if e != nil {
		t.Fatal(e)
	}
	currentProcess, e := RuntimeProcessID(ctx, fixtureSSH{}, provisioning.Target{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if VerifyRuntime(ctx, fixtureSSH{}, provisioning.Target{}, nil, verifiedPayload, after, currentProcess) == nil {
		t.Fatal("unchanged running process accepted")
	}
	obj, _ := decode(after)
	settings, _ := object(obj["settings"])
	var email string
	for _, v := range settings["clients"].([]any) {
		c := v.(map[string]any)
		if c["enable"] == true {
			email = stringField(c, "email")
			break
		}
	}
	out, e := exec.CommandContext(ctx, "lxc", "exec", "dobapply24", "--", "/usr/local/x-ui/bin/xray-linux-amd64", "api", "rmu", "--server=127.0.0.1:62789", "-tag="+stringField(obj, "tag"), email).CombinedOutput()
	if e != nil || !strings.Contains(string(out), "Removed 1") {
		t.Fatal("runtime fault injection failed")
	}
	if VerifyRuntime(ctx, fixtureSSH{}, provisioning.Target{}, nil, verifiedPayload, after, "different-process") == nil {
		t.Fatal("missing live user accepted despite unchanged startup file")
	}
	tx, _ = db.BeginTx(ctx, nil)
	if e = Rollback(ctx, tx, jobID); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = service.RunOne(ctx); e != nil {
		t.Fatal("native rollback", e)
	}
	restored, _, e := read(ctx, rt, testKey)
	if e != nil || Verify(actual, restored, testKey, old) != nil {
		t.Fatal("native rollback contract", e)
	}
	t.Log("PASS native Ubuntu24: old XHTTP -> host change + encrypted Vision + server/client FinalMask -> rollback; client identity/limits preserved; real core readback and executable validated")
}
