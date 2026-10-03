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
	fixtures := flag.Bool("fixture-delete", false, "exercise deletes on newly created disabled test fixtures only")
	flag.Parse()
	u, err := url.Parse(*base)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
		return errors.New("local API required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	if !*fixtures {
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
		{"INSERT INTO residential_proxies(proxy_id,outbound_tag,enabled) VALUES($1,$2,false)", []any{proxy, "residential-ads-" + proxy}},
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
	code, _, err = request("DELETE", "/api/v1/accounts/"+account, nil)
	if err != nil || code != 202 {
		return errors.New("account deletion journal unconfirmed")
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
	for _, table := range []string{"network_profiles", "account_billing_snapshots", "account_deletion_jobs", "secrets", "operations", "resources"} {
		if err = a.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE account_id=$1", account).Scan(&n); err != nil || n != 0 {
			return fmt.Errorf("fixture data remains in %s", table)
		}
	}
	fmt.Println("ACCOUNT_DURABLE_DELETE_AND_PURGE PASS")
	return nil
}
