package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func TestConfigProfileAdoptionPreservesNativeCredentials(t *testing.T) {
	db := adminTestDBThrough(t, 183)
	ctx := context.Background()
	var native map[string]any
	writes := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/csrf-token":
			fmt.Fprint(w, `{"success":true,"obj":"fixture"}`)
		case "/login":
			fmt.Fprint(w, `{"success":true}`)
		case "/panel/api/inbounds/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": []any{native}})
		default:
			writes++
			w.WriteHeader(500)
		}
	}))
	defer remote.Close()
	_, droplet, panel := seedPanel(t, db, remote.URL)
	sqlMust(t, db, "UPDATE deployments SET host='192.0.2.1' WHERE droplet_id=$1", droplet)
	sqlMust(t, db, "UPDATE trojan_tcp_policy SET enabled=true,fleet=true,revision=8")
	sqlMust(t, db, "INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound) VALUES('DIRECT','[443]',1)")
	var before string
	if e := db.QueryRow("SELECT row_to_json(p)::text FROM reality_config_profiles p").Scan(&before); e != nil {
		t.Fatal(e)
	}
	old, err := trojantcp.NewCredentials(panel)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := trojantcp.ReadPolicy(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	native, err = trojantcp.Build(policy, "192.0.2.1", old.Clients(policy))
	if err != nil {
		t.Fatal(err)
	}
	native["id"] = 7
	native["tag"] = "inbound-8443"
	secret, _ := json.Marshal(old)
	secrets := &trojanTestSecrets{values: map[string][]byte{"legacy-fixture": secret}}
	sqlMust(t, db, `INSERT INTO trojan_tcp_listeners(panel_id,port,remote_id,managed_key,credential_ref,policy_revision,state,certificate_not_after,verified_at)
 VALUES($1,8443,7,'dob:trojan-tcp:08443','legacy-fixture',8,'VERIFIED',now()+interval '5 days',now())`, panel)
	for _, cl := range old.Clients(policy) {
		sqlMust(t, db, "INSERT INTO trojan_tcp_clients(panel_id,route_class,client_id,email) VALUES($1,$2,$3,$4)", panel, cl.RouteClass, cl.ID, cl.Email)
		sqlMust(t, db, "INSERT INTO output_config_snapshots(panel_id,client_id,uri) VALUES($1,$2,$3)", panel, cl.ID, "trojan://"+cl.Password+"@192.0.2.1:8443?security=tls&type=tcp")
	}
	up, err := os.ReadFile("../../migrations/000184_config_profiles.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Use the production retention guard, including rollback of incomplete
	// adoption. Raw Exec would miss the installer's data-preservation contract.
	for _, mutation := range []string{
		"DELETE FROM config_driver_clients;",
		"UPDATE config_driver_profiles SET enabled=false;",
		"UPDATE config_driver_listeners SET credential_ref='incorrect';",
	} {
		dir := t.TempDir()
		file := filepath.Join(dir, "000184_config_profiles.up.sql")
		if e := os.WriteFile(file, append(append([]byte{}, up...), []byte("\n"+mutation)...), 0600); e != nil {
			t.Fatal(e)
		}
		e := (migrate.Runner{DB: db, Dir: dir, PreserveData: true}).Up(ctx)
		if e == nil || !strings.Contains(e.Error(), "profile adoption") {
			t.Fatal("incomplete adoption not rejected by retention", e)
		}
		var stillEnabled bool
		if e := db.QueryRow("SELECT enabled FROM trojan_tcp_policy").Scan(&stillEnabled); e != nil || !stillEnabled {
			t.Fatal("rejected migration changed old policy", e)
		}
	}
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "000184_config_profiles.up.sql"), up, 0600); e != nil {
		t.Fatal(e)
	}
	if e := (migrate.Runner{DB: db, Dir: dir, PreserveData: true}).Up(ctx); e != nil {
		t.Fatal("production-preserving migration", e)
	}

	const id = "00000000-0000-4000-8000-000000000001"
	p, err := configplane.Read(ctx, db, id)
	if err != nil || !p.Enabled || p.Revision != 8 || p.Spec.Port != 8443 {
		t.Fatal("adopted profile", err)
	}
	var enabled bool
	if e := db.QueryRow("SELECT enabled FROM trojan_tcp_policy").Scan(&enabled); e != nil || enabled {
		t.Fatal("two authorities")
	}
	var n int
	if e := db.QueryRow("SELECT count(*) FROM output_config_snapshots WHERE config_profile_id=$1 AND config_profile_revision=8", id).Scan(&n); e != nil || n != 2 {
		t.Fatal("snapshot revision migration", n, e)
	}
	service := configplane.Service{DB: db, Secrets: secrets, SSH: &trojanTestSSH{}, Runtimes: &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: secrets, Timeout: time.Second}},
		Verify: func(_ context.Context, _ provisioning.Target, _ []byte, payload map[string]any) error {
			if payload["tag"] != "inbound-8443" {
				t.Error("live tag lost")
			}
			return nil
		}, ProbeTLS: func(context.Context, string, int) (trojantcp.TLSProof, error) {
			return trojantcp.TLSProof{NotAfter: time.Now().Add(5 * 24 * time.Hour)}, nil
		}}
	if e := service.Reconcile(ctx, id, panel); e != nil {
		t.Fatal("adoption verification", e)
	}
	if writes != 0 || !reflect.DeepEqual(secrets.values["legacy-fixture"], secret) || len(secrets.values) != 1 {
		t.Fatal("migration changed native listener or credentials")
	}
	var after, state string
	if e := db.QueryRow("SELECT row_to_json(p)::text FROM reality_config_profiles p").Scan(&after); e != nil || before != after {
		t.Fatal("Reality changed", e)
	}
	if e := db.QueryRow("SELECT state FROM trojan_tcp_listeners").Scan(&state); e != nil || state != "RETIRED" {
		t.Fatal("legacy reservation not retired", e)
	}
	p.Spec.Port = 500
	if code, _ := profileSave(t, &Server{DB: db}, p, "PUT"); code != 200 {
		t.Fatal("independent low port refused")
	}
	// Even after desired port moves, the applied native port remains reserved.
	if _, e := db.Exec("UPDATE reality_config_profiles SET ports='[443,8443]'"); e == nil {
		t.Fatal("applied port stolen by Reality")
	}
	other := configplane.Profile{Name: "Other", Enabled: true, Fleet: true, Classes: []string{"DIRECT"}, Count: 1, Spec: configdriver.Spec{Protocol: "vmess", Network: "tcp", Security: "none", Port: 8443}}
	if code, _ := profileSave(t, &Server{DB: db}, other, "POST"); code != 409 {
		t.Fatal("applied port stolen by profile")
	}
	down, e := os.ReadFile("../../migrations/000184_config_profiles.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("ownership-destroying rollback accepted")
	}
	if _, e = configplane.Read(ctx, db, id); e != nil {
		t.Fatal("failed rollback damaged ownership", e)
	}
}
