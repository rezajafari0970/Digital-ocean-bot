package residentialsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net/url"
	"strings"
)

type Secrets interface {
	sanaei.SSHSecretReader
	GetProxy(context.Context, string, string) ([]byte, error)
}
type Service struct {
	DB      *sql.DB
	Secrets Secrets
	SSH     provisioning.SSHClient
}
type rp struct {
	ID, Type, Host, User, Tag string
	Port                      int
}

func (s Service) ReconcilePanel(ctx context.Context, p readyworker.Panel, dry bool) error {
	if s.DB == nil || s.Secrets == nil || p.ID == "" {
		return errors.New("residential sync config")
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT pr.id::text,pr.type,pr.host,pr.port,COALESCE(pr.username,''),rp.outbound_tag FROM residential_proxies rp JOIN proxies pr ON pr.id=rp.proxy_id WHERE rp.enabled=true AND pr.status='healthy' ORDER BY rp.priority,rp.proxy_id`)
	if e != nil {
		return e
	}
	defer rows.Close()
	var ps []rp
	for rows.Next() {
		var x rp
		if e = rows.Scan(&x.ID, &x.Type, &x.Host, &x.Port, &x.User, &x.Tag); e != nil {
			return e
		}
		ps = append(ps, x)
	}
	if dry {
		return nil
	}
	return s.apply(ctx, p.ID, ps)
}

func (s Service) apply(ctx context.Context, panelID string, ps []rp) error {
	var acc, did, host, user, keyref, puser, pref, path string
	var port int
	e := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref',''),x.username,x.password_secret_ref,x.port,x.web_path FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation WHERE pi.id=$1 AND pi.enabled=true`, panelID).Scan(&acc, &did, &host, &user, &keyref, &puser, &pref, &port, &path)
	if e != nil {
		return e
	}
	exec := sanaei.SSHSessionExecutorV2{SSH: s.SSH, Target: provisioning.Target{AccountID: acc, DropletID: did, Host: host, Port: 22, User: user, KeySecretRef: keyref}, PrivateKeySecretRef: keyref, PanelPasswordSecretRef: pref, AccountID: acc, Username: puser, Port: port, BasePath: path, DialHost: "127.0.0.1", Secrets: s.Secrets}
	cur, e := exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/", TimeoutSeconds: 10})
	if e != nil {
		return fmt.Errorf("residential read xray: %w", e)
	}
	var top struct {
		Success bool   `json:"success"`
		Obj     string `json:"obj"`
	}
	if json.Unmarshal(cur.Body, &top) != nil || !top.Success {
		return errors.New("xray read")
	}
	var inner struct {
		XraySetting     map[string]any `json:"xraySetting"`
		OutboundTestURL string         `json:"outboundTestUrl"`
	}
	if json.Unmarshal([]byte(top.Obj), &inner) != nil {
		return errors.New("xray parse")
	}
	outs, _ := inner.XraySetting["outbounds"].([]any)
	clean := make([]any, 0, len(outs)+len(ps))
	for _, raw := range outs {
		m, ok := raw.(map[string]any)
		if ok && strings.HasPrefix(fmt.Sprint(m["tag"]), "residential-ads-") {
			continue
		}
		clean = append(clean, raw)
	}
	for _, x := range ps {
		secret, e := s.Secrets.GetProxy(ctx, x.ID, "proxy-password")
		if e != nil {
			return e
		}
		proto := x.Type
		if proto == "socks5" {
			proto = "socks"
		}
		server := map[string]any{"address": x.Host, "port": x.Port}
		if x.User != "" || len(secret) > 0 {
			server["users"] = []any{map[string]any{"user": x.User, "pass": string(secret)}}
		}
		settings := map[string]any{"servers": []any{server}}
		if proto == "socks" {
			settings["udp"] = true
		}
		clean = append(clean, map[string]any{"tag": x.Tag, "protocol": proto, "settings": settings})
		for i := range secret {
			secret[i] = 0
		}
	}
	inner.XraySetting["outbounds"] = clean
	routing, _ := inner.XraySetting["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	rr := make([]any, 0, len(rules)+1)
	for _, raw := range rules {
		m, ok := raw.(map[string]any)
		if ok && fmt.Sprint(m["ruleTag"]) == "dob-residential-ads" {
			continue
		}
		rr = append(rr, raw)
	}
	if len(ps) > 0 {
		network := "tcp"
		if ps[0].Type == "socks5" {
			network = "tcp,udp"
		}
		rule := map[string]any{"type": "field", "ruleTag": "dob-residential-ads", "network": network, "domain": []string{"geosite:category-ads", "geosite:category-ads-all", "geosite:category-ads-ir", "domain:browserleaks.com"}, "outboundTag": ps[0].Tag}
		routing["rules"] = append([]any{rule}, rr...)
	} else {
		routing["rules"] = rr
	}
	inner.XraySetting["routing"] = routing
	next, _ := json.Marshal(inner.XraySetting)
	form := url.Values{}
	form.Set("xraySetting", string(next))
	if inner.OutboundTestURL != "" {
		form.Set("outboundTestUrl", inner.OutboundTestURL)
	}
	resp, e := exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/update", Body: []byte(form.Encode()), ContentType: "application/x-www-form-urlencoded", TimeoutSeconds: 20})
	if e != nil {
		if errors.Is(e, provisioning.ErrCommandOutcomeUnknown) {
			verified, verr := readXraySetting(ctx, exec)
			if verr == nil && desiredResidentialApplied(verified, ps) {
				return nil
			}
		}
		return fmt.Errorf("residential update xray: %w", e)
	}
	var saved struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if json.Unmarshal(resp.Body, &saved) != nil || !saved.Success {
		return fmt.Errorf("xray save: %s", saved.Msg)
	}
	return nil
}
