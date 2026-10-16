package panelbootstrap

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type nativeBootstrapSecrets struct{ sshReads, writes int }

func (s *nativeBootstrapSecrets) Get(_ context.Context, _ string, ref string) ([]byte, error) {
	if ref == "test" {
		return []byte("test"), nil
	}
	s.sshReads++
	return nil, errors.New("SSH unavailable; native API is healthy")
}
func (s *nativeBootstrapSecrets) Put(context.Context, string, string, string, []byte) error {
	s.writes++
	return nil
}
func TestNativeBootstrapConvergesWithoutSSH(t *testing.T) {
	for _, scenario := range []string{"healthy", "malformed_scan", "native_unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			malformed := scenario == "malformed_scan"
			unavailable := scenario == "native_unavailable"
			db := repairTestDB(t)
			scans := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/csrf-token":
					fmt.Fprint(w, `{"success":true,"obj":"csrf"}`)
				case "/login":
					if unavailable {
						w.WriteHeader(http.StatusUnauthorized)
						fmt.Fprint(w, `{"success":false}`)
						return
					}
					fmt.Fprint(w, `{"success":true}`)
				case "/panel/api/inbounds/list", "/panel/api/inbounds/list/slim":
					fmt.Fprint(w, `{"success":true,"obj":[]}`)
				case "/panel/api/server/status":
					fmt.Fprint(w, `{"success":true,"obj":{}}`)
				case "/panel/api/server/scanRealityTargets":
					scans++
					if malformed {
						fmt.Fprint(w, `{"success":false}`)
						return
					}
					if e := r.ParseForm(); e != nil {
						t.Error(e)
					}
					if v := r.Form.Get("targets"); v != "" && v != "www.example.com:443" {
						t.Errorf("unexpected target %s", v)
					}
					fmt.Fprint(w, `{"success":true,"obj":[{"target":"www.example.com:443","host":"www.example.com","port":443,"serverNames":["www.example.com"],"tlsVersion":"1.3","alpn":"h2","certValid":true,"certChainValid":true,"feasible":true,"latencyMs":10}]}`)
				default:
					t.Errorf("unexpected native route %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			account, _ := sanaei.UUIDv4()
			droplet, _ := sanaei.UUIDv4()
			panel, _ := sanaei.UUIDv4()
			sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')", account)
			sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES($1::uuid,$2,$3,'READY')", droplet, account, droplet)
			sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'test','{}')", panel, account)
			sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,host,profile_snapshot) VALUES(gen_random_uuid(),$1,$2,$3,'PANEL_COMPLETE','done','unreachable.invalid','{"ssh_user":"root","ssh_key_secret_ref":"ssh"}')`, account, panel, droplet)
			sqlMust(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES($1,$2,$3,'sanaei-3x-ui',$4,'test')", panel, account, droplet, server.URL)
			sqlMust(t, db, "INSERT INTO xui_panel_deployments(id,account_id,droplet_id,username,password_secret_ref,port,web_path,state) VALUES(gen_random_uuid(),$1,$2,'test','test',8080,'/','COMPLETED')", account, droplet)
			sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,sni_selection_mode) VALUES('reality','scored')")
			if unavailable {
				sqlMust(t, db, "INSERT INTO panel_runtime_repairs(panel_id) VALUES($1)", panel)
			}
			secrets := &nativeBootstrapSecrets{}
			svc := Service{DB: db, Secrets: secrets, ManagedKey: "dob:reality-primary:000001"}
			for i := 0; i < 4; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				err := svc.BootstrapPanel(ctx, readyworker.Panel{ID: panel})
				cancel()
				if unavailable {
					if err == nil || !strings.Contains(err.Error(), "SSH unavailable") {
						t.Fatalf("fallback lost: %v", err)
					}
				} else if malformed {
					if !errors.Is(err, sanaei.ErrRealityScan) {
						t.Fatalf("malformed scan got %v", err)
					}
				} else if i < 2 {
					if !errors.Is(err, ErrRealityWarming) {
						t.Fatalf("round %d: %v", i, err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
			if unavailable {
				if secrets.sshReads != 4 || secrets.writes != 0 {
					t.Fatalf("fallback calls: %+v", secrets)
				}
				return
			}
			var repairs int
			if e := db.QueryRow("SELECT count(*) FROM panel_runtime_repairs WHERE panel_id=$1", panel).Scan(&repairs); e != nil {
				t.Fatal(e)
			}
			if repairs != 0 {
				t.Fatal("healthy native API triggered runtime repair")
			}
			if secrets.sshReads != 0 {
				t.Fatalf("healthy native bootstrap requested SSH %d times", secrets.sshReads)
			}
			var selections int
			if e := db.QueryRow("SELECT count(*) FROM reality_target_selections WHERE panel_id=$1", panel).Scan(&selections); e != nil {
				t.Fatal(e)
			}
			if malformed {
				if selections != 0 || secrets.writes != 0 {
					t.Fatal("invalid scan persisted selection/credentials")
				}
				return
			}
			if selections != 1 || scans != 6 || secrets.writes != 2 {
				t.Fatal(selections, scans, secrets.writes)
			}
			var public string
			if e := db.QueryRow("SELECT public_key FROM reality_credentials WHERE panel_id=$1", panel).Scan(&public); e != nil {
				t.Fatal(e)
			}
			b, e := base64.RawURLEncoding.DecodeString(strings.TrimSpace(public))
			if e != nil || len(b) != 32 {
				t.Fatal("invalid generated X25519 key")
			}
		})
	}
}
