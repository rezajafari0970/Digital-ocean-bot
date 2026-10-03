package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

const quotaBytes int64 = 2 * 1024 * 1024

func quotaCallbackIP(host string) error {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() {
		return fmt.Errorf("quota callback requires a local public IPv4")
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return err
	}
	for _, a := range addrs {
		n, _, e := net.ParseCIDR(a.String())
		if e == nil && n.Equal(ip) {
			return nil
		}
	}
	return fmt.Errorf("quota callback IP is not assigned to this control server")
}
func quotaXrayConfig(uri string, port int) ([]byte, string, error) {
	u, e := url.Parse(uri)
	if e != nil || u.User == nil {
		return nil, "", fmt.Errorf("invalid quota client URI")
	}
	q := u.Query()
	remotePort, e := strconv.Atoi(u.Port())
	if u.Scheme != "vless" || e != nil || remotePort < 1 || remotePort > 65535 || net.ParseIP(u.Hostname()) == nil || q.Get("security") != "reality" || q.Get("type") != "tcp" || q.Get("sni") == "" || q.Get("pbk") == "" || q.Get("sid") == "" || q.Get("flow") != "xtls-rprx-vision" || port < 1 || port > 65535 {
		return nil, "", fmt.Errorf("unsupported quota client configuration")
	}
	config := map[string]any{
		"log":      map[string]any{"loglevel": "none"},
		"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "http", "settings": map[string]any{}}},
		"outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": u.Hostname(), "port": remotePort, "users": []any{map[string]any{"id": u.User.Username(), "encryption": "none", "flow": q.Get("flow")}}}}},
			"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": q.Get("sni"), "fingerprint": q.Get("fp"), "publicKey": q.Get("pbk"), "shortId": q.Get("sid")}}}},
	}
	b, e := json.Marshal(config)
	return b, u.Hostname(), e
}

type quotaTraffic struct {
	Client   *http.Client
	URL      string
	Requests atomic.Int64
	Served   atomic.Int64
	close    func()
}

func startQuotaTraffic(ctx context.Context, uri, host string) (*quotaTraffic, error) {
	if err := quotaCallbackIP(host); err != nil {
		return nil, err
	}
	local, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := local.Addr().(*net.TCPAddr).Port
	config, remote, err := quotaXrayConfig(uri, port)
	local.Close()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("/root/backups/dob-admin-routing-20261003", "quota-xray-")
	if err != nil {
		return nil, err
	}
	os.Chmod(dir, 0700)
	cleanupDir := true
	defer func() {
		if cleanupDir {
			os.RemoveAll(dir)
		}
	}()
	path := filepath.Join(dir, "client.json")
	if err = os.WriteFile(path, config, 0600); err != nil {
		return nil, err
	}
	test := exec.CommandContext(ctx, "/usr/local/bin/xray", "run", "-test", "-config", path)
	if err = test.Run(); err != nil {
		return nil, fmt.Errorf("installed Xray rejected quota client config")
	}
	childCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(childCtx, "/usr/local/bin/xray", "run", "-config", path)
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	alive := false
	for i := 0; i < 40; i++ {
		select {
		case <-finished:
			cancel()
			return nil, fmt.Errorf("quota Xray exited before ready")
		default:
		}
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if e == nil {
			c.Close()
			alive = true
			break
		}
		select {
		case <-ctx.Done():
			cancel()
			<-finished
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !alive {
		cancel()
		<-finished
		return nil, fmt.Errorf("quota Xray local listener unavailable")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(host, "0"))
	if err != nil {
		cancel()
		<-finished
		return nil, err
	}
	nonce, err := sanaei.UUIDv4()
	if err != nil {
		listener.Close()
		cancel()
		<-finished
		return nil, err
	}
	proxyURL, _ := url.Parse("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second}
	t := &quotaTraffic{Client: &http.Client{Transport: transport, Timeout: 20 * time.Second}, URL: "http://" + listener.Addr().String() + "/" + nonce}
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil || !net.ParseIP(source).Equal(net.ParseIP(remote)) || r.URL.Path != "/"+nonce || r.Method != "GET" {
			http.Error(w, "denied", 403)
			return
		}
		if t.Requests.Add(1) > 4 {
			http.Error(w, "budget exhausted", 429)
			return
		}
		const n = 6 * 1024 * 1024
		w.Header().Set("Content-Length", strconv.Itoa(n))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Quota-Canary", nonce)
		block := make([]byte, 32768)
		for sent := 0; sent < n; {
			written, e := w.Write(block)
			sent += written
			t.Served.Add(int64(written))
			if e != nil {
				return
			}
		}
	})
	go server.Serve(listener)
	cleanupDir = false
	t.close = func() { transport.CloseIdleConnections(); server.Close(); cancel(); <-finished; os.RemoveAll(dir) }
	return t, nil
}
func (t *quotaTraffic) transfer(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", t.URL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("quota test tunnel request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Quota-Canary") == "" {
		return 0, fmt.Errorf("quota test callback not reached")
	}
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 6*1024*1024+1))
	if n > 6*1024*1024 {
		return n, fmt.Errorf("traffic budget exceeded")
	}
	return n, err
}

func quotaPhase(ctx context.Context, db *sql.DB, rt *sanaei.PanelRuntime, j clientops.Journal, r scaleRun, original map[string]sanaei.Client, host string) error {
	if _, err := db.ExecContext(ctx, "UPDATE bulk_lifecycle_scopes SET quota_bytes=$2,lifetime_seconds=600,device_limit=3,updated_at=now() WHERE generation_id=$1", r.ID, quotaBytes); err != nil {
		return err
	}
	if err := lifeWait(ctx, j, "quota policy update", func() (bool, error) {
		obs, _, err := clientops.LifecycleInventory(ctx, rt, r.Inbound)
		if errors.Is(err, clientops.ErrVerify) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		for id := range original {
			v, ok := obs[id]
			if !ok || v.Client.TotalGB != quotaBytes || v.Client.LimitHWID != 3 || v.Client.ExpiryTime < time.Now().Add(5*time.Minute).UnixMilli() {
				return false, nil
			}
		}
		_, _, pending, err := lifeCounts(ctx, db, r.ID)
		return pending == 0, err
	}); err != nil {
		return err
	}
	// Pause this generation so the exhausted record remains observable until proof.
	if _, err := db.ExecContext(ctx, "UPDATE bulk_lifecycle_scopes SET enabled=false,updated_at=now() WHERE generation_id=$1", r.ID); err != nil {
		return err
	}
	var clientID, uri string
	if err := lifeWait(ctx, j, "fresh direct quota output", func() (bool, error) {
		err := db.QueryRowContext(ctx, `SELECT o.client_id,s.uri FROM bulk_user_ownership o JOIN output_config_snapshots s ON s.client_id=o.client_id JOIN panel_client_routes m ON m.panel_id=s.panel_id AND m.client_id=s.client_id JOIN panel_routing_state r ON r.panel_id=s.panel_id CROSS JOIN residential_routing_control c
  WHERE o.generation_id=$1 AND o.state='ACTIVE' AND s.panel_id=$2 AND s.visible_until>now() AND s.last_seen_at>now()-interval '15 seconds' AND m.effective_class='DIRECT' AND m.route_class='DIRECT' AND m.revision=c.revision AND r.revision=c.revision AND r.state='APPLIED' AND r.verified_at>now()-interval '45 seconds' AND c.enabled AND (c.fleet OR r.panel_id=ANY(c.panel_ids)) ORDER BY o.client_id LIMIT 1`, r.ID, r.Panel).Scan(&clientID, &uri)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil, err
	}); err != nil {
		return err
	}
	if _, ok := original[clientID]; !ok {
		return fmt.Errorf("quota identity outside planned generation")
	}
	before, _, err := clientops.LifecycleInventory(ctx, rt, r.Inbound)
	if err != nil {
		return err
	}
	base := before[clientID]
	if base.Traffic == nil || base.Traffic.Up+base.Traffic.Down != 0 {
		return fmt.Errorf("quota test requires unused owned client")
	}
	traffic, err := startQuotaTraffic(ctx, uri, host)
	if err != nil {
		return err
	}
	defer traffic.close()
	received, transferErr := traffic.transfer(ctx)
	if received < quotaBytes {
		return fmt.Errorf("quota traffic below threshold bytes=%d callback_requests=%d", received, traffic.Requests.Load())
	}
	_ = transferErr // A quota cutoff can terminate the in-flight response.
	var up, down int64
	if err = lifeWait(ctx, j, "provider quota counters", func() (bool, error) {
		obs, _, e := clientops.LifecycleInventory(ctx, rt, r.Inbound)
		if errors.Is(e, clientops.ErrVerify) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		c, ok := obs[clientID]
		if !ok || c.Traffic == nil {
			return false, fmt.Errorf("quota record missing before evidence")
		}
		up, down = c.Traffic.Up, c.Traffic.Down
		return up >= quotaBytes || down >= quotaBytes-up, nil
	}); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||jsonb_build_object('acceptance','REAL_QUOTA','quota_client_id',$2::text,'quota_bytes',$3::bigint,'observed_up',$4::bigint,'observed_down',$5::bigint,'traffic_received',$6::bigint,'traffic_served',$7::bigint,'quota_observed_at',now()) WHERE generation_id=$1`, r.ID, clientID, quotaBytes, up, down, received, traffic.Served.Load()); err != nil {
		return err
	}
	fmt.Printf("QUOTA_TRAFFIC_PROVEN run=%s quota=%d up=%d down=%d received=%d\n", r.ID, quotaBytes, up, down, received)
	// The only next mutation is journalled worker cleanup and one replacement.
	if _, err = db.ExecContext(ctx, "UPDATE bulk_lifecycle_scopes SET enabled=true,updated_at=now() WHERE generation_id=$1", r.ID); err != nil {
		return err
	}
	if err = lifeWait(ctx, j, "quota deletion and one replacement", func() (bool, error) {
		active, deleted, pending, e := lifeCounts(ctx, db, r.ID)
		return active == 3 && deleted == 1 && pending == 0, e
	}); err != nil {
		return err
	}
	all, err := scaleExpected(ctx, db, r)
	if err != nil {
		return err
	}
	if len(all) != 4 {
		return fmt.Errorf("quota replacement identity count=%d", len(all))
	}
	obs, _, err := clientops.LifecycleInventory(ctx, rt, r.Inbound)
	if err != nil {
		return err
	}
	if _, ok := obs[clientID]; ok {
		return fmt.Errorf("quota exhausted identity still present")
	}
	for id := range all {
		if id == clientID {
			continue
		}
		c, ok := obs[id]
		if !ok || c.Client.TotalGB != quotaBytes || c.Client.LimitHWID != 3 || !c.Client.Enable {
			return fmt.Errorf("replacement/remaining policy mismatch")
		}
	}
	if err = scaleOutput(ctx, db, r, all, 3); err != nil {
		return err
	}
	var deletes int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE payload->>'GenerationID'=$1 AND kind='BULK_DELETE' AND state='SUCCEEDED' AND payload->'CleanupReasons'->>$2 IN('QUOTA','DISABLED')`, r.ID, clientID).Scan(&deletes); err != nil || deletes != 1 {
		return fmt.Errorf("quota cleanup journal not unique count=%d error=%v", deletes, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE bulk_scale_runs SET phase='CLEANING',evidence=evidence||jsonb_build_object('quota_deleted',1,'quota_replacements',1,'replacement_output_verified',true) WHERE generation_id=$1`, r.ID); err != nil {
		return err
	}
	fmt.Printf("QUOTA_REPLACEMENT_PROVEN run=%s deleted=1 replacement=1\n", r.ID)
	return nil
}
