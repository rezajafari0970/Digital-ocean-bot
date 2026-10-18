package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type profileFixtureSecrets struct{ trojanTestSecrets }

func (s *profileFixtureSecrets) Get(ctx context.Context, account, key string) ([]byte, error) {
	if key == "test" || key == "" {
		return []byte("fixture-only-disposable-password"), nil
	}
	return s.trojanTestSecrets.Get(ctx, account, key)
}

type profileFixtureSSH struct{}

func (profileFixtureSSH) Run(ctx context.Context, _ provisioning.Target, _ []byte, command string) (string, error) {
	out, e := exec.CommandContext(ctx, "lxc", "exec", "dobtest24", "--", "sh", "-c", command).CombinedOutput()
	if strings.Contains(command, "DOB_PROFILE_VERIFY") {
		fmt.Printf("profile runtime result: %s", out)
	}
	return string(out), e
}
func (profileFixtureSSH) RunInput(ctx context.Context, _ provisioning.Target, _ []byte, command string, input []byte) (provisioning.CommandResult, error) {
	var want map[string]any
	if e := json.Unmarshal(input, &want); e != nil {
		return provisioning.CommandResult{}, e
	}
	cert, e := exec.CommandContext(ctx, "lxc", "exec", "dobtest24", "--", "cat", "/opt/dob-trojan/tls/fullchain.pem").Output()
	if e != nil {
		return provisioning.CommandResult{}, e
	}
	for _, v := range want["probes"].([]any) {
		stream := v.(map[string]any)["streamSettings"].(map[string]any)
		if stream["security"] == "tls" {
			stream["tlsSettings"].(map[string]any)["certificates"] = []any{map[string]any{"usage": "verify", "certificate": strings.Split(strings.TrimSpace(string(cert)), "\n")}}
		}
	}
	input, _ = json.Marshal(want)
	cmd := exec.CommandContext(ctx, "lxc", "exec", "dobtest24", "--", "sh", "-c", command)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	e = cmd.Run()
	fmt.Printf("profile runtime result: %s", stdout.String())
	return provisioning.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}, e
}
func TestNativeProfileControlPlane(t *testing.T) {
	base := os.Getenv("DOB_NATIVE_PROFILE_URL")
	if base == "" {
		t.Skip("explicit disposable native fixture required")
	}
	if base != "http://10.231.74.113:2053/fixture" {
		t.Fatal("refusing non-fixture panel")
	}
	db := adminTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	_, droplet, panel := seedPanel(t, db, base)
	sqlMust(t, db, "UPDATE deployments SET host='10.231.74.113' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE xui_panel_deployments SET username='fixture',port=2053 WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL")
	secrets := &profileFixtureSecrets{trojanTestSecrets{values: map[string][]byte{}}}
	rt := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: 5 * time.Second}}
	service := configplane.Service{DB: db, Secrets: secrets, Runtimes: rt, SSH: profileFixtureSSH{}, ProbeTLS: func(context.Context, string, int) (trojantcp.TLSProof, error) {
		return trojantcp.TLSProof{NotAfter: time.Now().Add(5 * 24 * time.Hour)}, nil
	}}
	catalog := configdriver.Builtins().Catalog()
	for i, cap := range catalog {
		if !cap.Public {
			continue
		}
		t.Run(strings.ReplaceAll(cap.ID, "/", "_"), func(t *testing.T) {
			spec := configdriver.Spec{Protocol: cap.Protocol, Network: cap.Network, Security: cap.Security, Port: 19400 + i}
			if spec.Network == "ws" || spec.Network == "xhttp" {
				spec.Path = "/profile-fixture"
			}
			if spec.Network == "grpc" {
				spec.Path = "profilefixture"
			}
			if spec.Security == "reality" {
				spec.SNI = "www.microsoft.com"
				if spec.Network == "tcp" {
					spec.Flow = "xtls-rprx-vision"
				}
			}
			p := configplane.Profile{Name: "Native fixture", Enabled: true, Fleet: false, Panels: []string{panel}, Classes: []string{"DIRECT", "RESIDENTIAL"}, Count: 1, Spec: spec}
			code, p := profileSave(t, &Server{DB: db}, p, "POST")
			if code != 200 {
				t.Fatal(code)
			}
			raw, _ := json.Marshal(spec)
			sqlMust(t, db, "INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec,certificate_not_after) VALUES($1,$2,$3,$4,$5,now()+interval '5 days')", p.ID, panel, configdriver.ManagedPrefix+p.ID, "fixture:"+p.ID, raw)
			if e := service.Reconcile(ctx, p.ID, panel); e != nil {
				t.Fatal("native create", e)
			}
			var id int64
			if e := db.QueryRow("SELECT remote_id FROM config_driver_listeners WHERE profile_id=$1", p.ID).Scan(&id); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
				defer stop()
				session, e := rt.Acquire(cleanup, panel)
				if e != nil {
					t.Error(e)
					return
				}
				if e = sanaei.DeleteInbound(cleanup, session.Session.Exec, id); e != nil {
					t.Error("fixture cleanup", e)
				}
			})
			p.Spec.Port += 100
			code, p = profileSave(t, &Server{DB: db}, p, "PUT")
			if code != 200 {
				t.Fatal(code)
			}
			if e := service.Reconcile(ctx, p.ID, panel); e != nil {
				t.Fatal("native port change", e)
			}
			if e := service.Reconcile(ctx, p.ID, panel); e != nil {
				t.Fatal("native fresh verify", e)
			}
		})
	}
}
