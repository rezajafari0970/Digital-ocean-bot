package sanaei

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrRouteBatchUnavailable = errors.New("native route batch unavailable")

//go:embed route_batch.py
var routeBatchScript string
var routeBatchSlots = make(chan struct{}, 8)

func (s *ResilientSession) RouteProofBatch(ctx context.Context, requests []SessionRequest) ([]SessionResponse, error) {
	if s.BatchRouteReads == nil {
		return nil, ErrRouteBatchUnavailable
	}
	return s.BatchRouteReads(ctx, requests)
}
func routeBatchForms(requests []SessionRequest) ([]string, error) {
	if len(requests) == 0 || len(requests) > 256 {
		return nil, ErrRouteBatchUnavailable
	}
	out := make([]string, len(requests))
	allowed := map[string]bool{"port": true, "network": true, "inboundTag": true, "email": true, "protocol": true, "domain": true, "ip": true}
	for i, r := range requests {
		if r.Method != "POST" || r.Path != "panel/api/xray/routeTest" || r.ContentType != "application/x-www-form-urlencoded" || len(r.Body) > 4096 {
			return nil, ErrRouteBatchUnavailable
		}
		form, e := url.ParseQuery(string(r.Body))
		if e != nil {
			return nil, ErrRouteBatchUnavailable
		}
		for key, values := range form {
			if !allowed[key] || len(values) != 1 {
				return nil, ErrRouteBatchUnavailable
			}
		}
		out[i] = string(r.Body)
	}
	return out, nil
}
func parseRouteBatch(raw string, count int) ([]SessionResponse, error) {
	var data struct {
		Error   string
		Results []struct {
			Index, Status              int
			Success, Matched, Starting bool
			Tag                        string
		}
	}
	if json.Unmarshal([]byte(raw), &data) != nil || data.Error != "" || len(data.Results) != count {
		return nil, ErrRouteBatchUnavailable
	}
	out := make([]SessionResponse, count)
	for i, v := range data.Results {
		if v.Index != i || v.Status < 100 || v.Status > 599 || len(v.Tag) > 256 {
			return nil, ErrRouteBatchUnavailable
		}
		msg := ""
		if v.Starting {
			msg = "rpc error: code = Unavailable"
		}
		body, _ := json.Marshal(map[string]any{"success": v.Success, "msg": msg, "obj": map[string]any{"matched": v.Matched, "outboundTag": v.Tag}})
		out[i] = SessionResponse{StatusCode: v.Status, Body: body}
	}
	return out, nil
}
func (f RuntimeFactory) routeProofBatch(ctx context.Context, panel string, client *APIClient, requests []SessionRequest) ([]SessionResponse, error) {
	forms, e := routeBatchForms(requests)
	if e != nil {
		return nil, e
	}
	if client == nil || client.HTTP == nil || client.HTTP.Jar == nil {
		return nil, ErrRouteBatchUnavailable
	}
	select {
	case routeBatchSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-routeBatchSlots }()
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var target provisioning.Target
	var panelPort int
	e = f.DB.QueryRowContext(bounded, `SELECT p.account_id::text,p.droplet_id::text,dep.host,COALESCE(dep.profile_snapshot->>'ssh_user','root'),COALESCE(dep.profile_snapshot->>'ssh_key_secret_ref',''),x.port
 FROM panel_instances p JOIN deployments dep ON dep.droplet_id=p.droplet_id JOIN droplets dr ON dr.id=p.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=dr.id AND x.generation=dep.postinstall_generation
 WHERE p.id=$1 AND p.enabled AND dep.state='PANEL_COMPLETE' AND dr.state IN('READY','EXPIRING')`, panel).Scan(&target.AccountID, &target.DropletID, &target.Host, &target.User, &target.KeySecretRef, &panelPort)
	if e != nil || target.KeySecretRef == "" {
		return nil, ErrRouteBatchUnavailable
	}
	u, e := url.Parse(client.BaseURL)
	if e != nil || u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || u.Hostname() != target.Host || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, ErrRouteBatchUnavailable
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port != panelPort {
		return nil, ErrRouteBatchUnavailable
	}
	endpoint := strings.TrimRight(u.EscapedPath(), "/") + "/"
	if strings.ContainsAny(endpoint, "\r\n?#") || strings.Contains(endpoint, "..") {
		return nil, ErrRouteBatchUnavailable
	}
	if e = client.lockAuth(bounded, false); e != nil {
		return nil, e
	}
	req, _ := http.NewRequest("POST", client.BaseURL+"/panel/api/xray/routeTest", nil)
	for _, cookie := range client.HTTP.Jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
	cookie, csrf := req.Header.Get("Cookie"), client.CSRF
	client.authMu.RUnlock()
	if cookie == "" || csrf == "" {
		return nil, ErrRouteBatchUnavailable
	}
	deadline, _ := bounded.Deadline()
	input, e := json.Marshal(map[string]any{"port": port, "path": endpoint, "cookie": cookie, "csrf": csrf, "forms": forms, "deadline": float64(deadline.Add(-100*time.Millisecond).UnixMilli()) / 1000})
	if e != nil {
		return nil, ErrRouteBatchUnavailable
	}
	defer wipeAuth(input)
	key, e := f.Secrets.Get(bounded, target.AccountID, target.KeySecretRef)
	if e != nil {
		return nil, ErrRouteBatchUnavailable
	}
	defer wipeAuth(key)
	target.Port = 22
	// Session credentials are stdin data, never shell arguments or node files.
	command := "python3 -c 'import base64;exec(base64.b64decode(\"" + base64.StdEncoding.EncodeToString([]byte(routeBatchScript)) + "\"))'"
	ssh := provisioning.SSHClient{Timeout: 3 * time.Second, OutputLimit: 65536, HostKeys: provisioning.SQLHostKeyPins{DB: f.DB}}
	result, e := ssh.RunInput(bounded, target, key, command, input)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrRouteBatchUnavailable
	}
	return parseRouteBatch(result.Stdout, len(requests))
}
