package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	base := flag.String("base-url", "http://127.0.0.1:18080", "local production API")
	localPurge := flag.Bool("fixture-local-purge", false, "exercise explicit local purge on a newly created blocked fixture only")
	fixtures := flag.Bool("fixture-delete", false, "exercise deletes on newly created disabled test fixtures only")
	browserScript := flag.String("browser-script", "", "run a local browser acceptance script with an ephemeral session")
	fixtureBrowser := flag.String("fixture-browser-script", "", "exercise the new fixture account Delete button in a local browser")
	residentialFixture := flag.Bool("residential-create-fixture", false, "create and delete a disabled isolated residential fixture using an existing verified endpoint")
	flag.Parse()
	u, err := url.Parse(*base)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
		return errors.New("local API required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.Close()
	user, _ := sanaei.UUIDv4()
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	token := hex.EncodeToString(secret)
	sum := sha256.Sum256([]byte(token))
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO admin_users(id,username,password_hash,role) VALUES($1,$2,'!','admin')", user, "acceptance-"+user); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO admin_sessions(id,user_id,token_hash,expires_at) VALUES(gen_random_uuid(),$1,$2,now()+interval '2 minutes')", user, sum[:]); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	defer func() {
		c, x := context.WithTimeout(context.Background(), 5*time.Second)
		defer x()
		if _, e := a.DB.ExecContext(c, "DELETE FROM admin_users WHERE id=$1 AND username=$2", user, "acceptance-"+user); e != nil {
			fmt.Fprintln(os.Stderr, "temporary acceptance session cleanup failed")
		}
	}()
	client := &http.Client{Timeout: 10 * time.Second}
	request := func(method, path string, body []byte) (int, []byte, error) {
		req, e := http.NewRequestWithContext(ctx, method, *base+path, bytes.NewReader(body))
		if e != nil {
			return 0, nil, e
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			return 0, nil, errors.New("local HTTP outcome unknown; inspect fixture state before retry")
		}
		defer resp.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return resp.StatusCode, raw, e
	}
	if *browserScript != "" {
		code, raw, e := request("POST", "/api/v1/output/share", []byte(`{"route_class":"DIRECT"}`))
		var share struct{ Token, URL string }
		if e != nil || code != 200 || json.Unmarshal(raw, &share) != nil || share.Token == "" {
			return errors.New("browser output fixture failed")
		}
		defer func() {
			c, x := context.WithTimeout(context.Background(), 5*time.Second)
			defer x()
			a.DB.ExecContext(c, "DELETE FROM output_share_tokens WHERE token=$1", share.Token)
		}()
		command := exec.CommandContext(ctx, "node", *browserScript)
		command.Env = append(os.Environ(), "DOB_UI_TOKEN="+token, "DOB_UI_BASE="+*base, "DOB_UI_OUTPUT_URL="+*base+share.URL+"?view=1")
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err = command.Run(); err != nil {
			return errors.New("browser acceptance failed")
		}
	}
	for _, path := range []string{"/api/v1/system", "/api/v1/accounts", "/api/v1/proxies", "/api/v1/residential-proxies", "/api/v1/residential-routing", "/api/v1/configs", "/api/v1/config-capacity", "/api/v1/output?route_class=DIRECT", "/api/v1/output?route_class=RESIDENTIAL"} {
		code, raw, e := request("GET", path, nil)
		if e != nil {
			return e
		}
		if code != 200 {
			return fmt.Errorf("API acceptance failed: %s status=%d", path, code)
		}
		if path == "/api/v1/system" || path == "/api/v1/residential-routing" {
			fmt.Printf("%s %s\n", path, raw)
		} else {
			fmt.Printf("GET %s status=200 bytes=%d\n", path, len(raw))
		}
	}
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		body, _ := json.Marshal(map[string]string{"route_class": class})
		code, raw, e := request("POST", "/api/v1/output/share", body)
		if e != nil || code != 200 {
			return errors.New("share creation failed")
		}
		var share struct{ Token, URL string }
		if json.Unmarshal(raw, &share) != nil || share.Token == "" {
			return errors.New("share response invalid")
		}
		code, _, e = request("GET", share.URL+"?route_class=ALL", nil)
		_, delErr := a.DB.ExecContext(ctx, "DELETE FROM output_share_tokens WHERE token=$1 AND route_class=$2", share.Token, class)
		if e != nil || code != 400 || delErr != nil {
			return errors.New("share class boundary or cleanup failed")
		}
		fmt.Printf("SHARE_CLASS_BOUNDARY %s PASS\n", class)
	}
	if *residentialFixture {
		var endpoint, typ, host, user, ref string
		var port int
		err = a.DB.QueryRowContext(ctx, "SELECT proxy_id::text,type,host,port,COALESCE(username,''),COALESCE(secret_ref,'') FROM residential_proxies WHERE enabled AND status='healthy' AND last_success_at>now()-interval '3 minutes' ORDER BY priority LIMIT 1").Scan(&endpoint, &typ, &host, &port, &user, &ref)
		if err != nil {
			return errors.New("no verified residential endpoint for fixture")
		}
		var password []byte
		if ref != "" {
			password, err = a.Container.Secrets.GetResidential(ctx, endpoint, ref)
			if err != nil {
				return err
			}
		}
		nameID, _ := sanaei.UUIDv4()
		body, _ := json.Marshal(map[string]any{"name": "acceptance-" + nameID, "type": typ, "host": host, "port": port, "username": user, "password": string(password), "priority": 999999, "enabled": false})
		for i := range password {
			password[i] = 0
		}
		code, raw, e := request("POST", "/api/v1/residential-proxies", body)
		for i := range body {
			body[i] = 0
		}
		if e != nil || code != 201 {
			return errors.New("residential create outcome unconfirmed; inspect acceptance fixture before retry")
		}
		var created struct{ ID string }
		if json.Unmarshal(raw, &created) != nil || created.ID == "" {
			return errors.New("residential fixture identity missing")
		}
		fmt.Printf("RESIDENTIAL_FIXTURE id=%s\\n", created.ID)
		var n int
		if err = a.DB.QueryRowContext(ctx, "SELECT count(*) FROM proxies WHERE id=$1", created.ID).Scan(&n); err != nil || n != 0 {
			return errors.New("residential create leaked into proxies")
		}
		got, e := a.Container.Secrets.GetResidential(ctx, created.ID, "proxy-password")
		if ref != "" && (e != nil || len(got) == 0) {
			return errors.New("residential fixture credential not readable")
		}
		for i := range got {
			got[i] = 0
		}
		code, _, e = request("DELETE", "/api/v1/residential-proxies/"+created.ID, nil)
		if e != nil || code != 204 {
			return errors.New("residential fixture deletion unconfirmed")
		}
		if err = a.DB.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM residential_proxies WHERE proxy_id=$1)+(SELECT count(*) FROM residential_proxy_secrets WHERE residential_id=$1)", created.ID).Scan(&n); err != nil || n != 0 {
			return errors.New("residential fixture data remains")
		}
		fmt.Println("RESIDENTIAL_API_CREATE_SECRET_ISOLATION_DELETE PASS")
	}
	if !*fixtures && !*localPurge {
		return nil
	}
	account, _ := sanaei.UUIDv4()
	proxy, _ := sanaei.UUIDv4()
	tx, err = a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO accounts(id,name,provider,secret_ref,enabled) VALUES($1,$2,'digitalocean','fixture',false)", []any{account, "acceptance-" + account}},
		{"INSERT INTO proxies(id,name,type,host,port,status) VALUES($1,$2,'socks5','127.0.0.1',9,'down')", []any{proxy, "acceptance-" + proxy}},
		{"INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,'proxy_required',$2)", []any{account, proxy}},
		{"INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag,enabled) VALUES($1,'test','socks5','localhost',1080,$2,false)", []any{proxy, "residential-ads-" + proxy}},
		{"INSERT INTO account_billing_snapshots(account_id,data) VALUES($1,'{}')", []any{account}},
	} {
		if _, err = tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Fixture identity is printed for read-before-write recovery; no credentials.
	fmt.Printf("FIXTURE account=%s proxy=%s\n", account, proxy)
	code, _, err := request("DELETE", "/api/v1/residential-proxies/"+proxy, nil)
	if err != nil || code != 204 {
		return errors.New("residential fixture delete unconfirmed")
	}
	var n int
	if err = a.DB.QueryRowContext(ctx, "SELECT count(*) FROM proxies p JOIN network_profiles np ON np.proxy_id=p.id WHERE p.id=$1 AND np.account_id=$2", proxy, account).Scan(&n); err != nil || n != 1 {
		return errors.New("shared account proxy was not preserved")
	}
	fmt.Println("RESIDENTIAL_SHARED_DELETE PASS")
	code, _, err = request("DELETE", "/api/v1/proxies/"+proxy, nil)
	if err != nil || code != 204 {
		return errors.New("proxy fixture delete unconfirmed")
	}
	var mode string
	var missing bool
	if err = a.DB.QueryRowContext(ctx, "SELECT mode,proxy_id IS NULL FROM network_profiles WHERE account_id=$1", account).Scan(&mode, &missing); err != nil || mode != "proxy_required" || !missing {
		return errors.New("proxy detach violated fail-closed mode")
	}
	fmt.Println("PROXY_FAIL_CLOSED_DELETE PASS")
	code, _, err = request("GET", "/api/v1/accounts/"+account+"/dashboard", nil)
	if err != nil || code != 200 {
		return errors.New("account detail failed")
	}

	if *localPurge {
		if *fixtureBrowser == "" {
			return errors.New("local purge fixture requires actual browser")
		}
		tx, err := a.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, "UPDATE accounts SET provider_state='LOCKED',runtime_status='DELETE_PENDING',deletion_requested_at=now()-interval '3 minutes' WHERE id=$1 AND NOT enabled", account); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO account_deletion_jobs(account_id,next_attempt_at) VALUES($1,now()+interval '1 hour')", account); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,$2,'RETIRING')", account, "acceptance-"+account); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if *fixtureBrowser != "" {
		command := exec.CommandContext(ctx, "node", *fixtureBrowser)
		command.Env = append(os.Environ(), "DOB_UI_TOKEN="+token, "DOB_UI_BASE="+*base, "DOB_UI_FIXTURE_ACCOUNT="+account, fmt.Sprintf("DOB_UI_EXPECT_LOCAL_PURGE=%t", *localPurge))
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err = command.Run(); err != nil {
			return errors.New("fixture browser deletion unconfirmed")
		}
	} else {
		code, _, err = request("DELETE", "/api/v1/accounts/"+account, nil)
		if err != nil || code != 202 {
			return errors.New("account deletion journal unconfirmed")
		}
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err = a.DB.QueryRowContext(ctx, "SELECT count(*) FROM accounts WHERE id=$1", account).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return errors.New("fixture deletion remains pending")
		case <-ticker.C:
		}
	}
	for _, table := range []string{"network_profiles", "account_billing_snapshots", "account_deletion_jobs", "secrets", "operations", "resources", "droplets", "deployments", "panel_instances"} {
		if err = a.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE account_id=$1", account).Scan(&n); err != nil || n != 0 {
			return fmt.Errorf("fixture data remains in %s", table)
		}
	}
	fmt.Println("ACCOUNT_DURABLE_DELETE_AND_PURGE PASS")
	return nil
}
