package usercapacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}
type Service struct {
	DB      *sql.DB
	Secrets Secrets
}
type rawInbound struct {
	ID             int               `json:"id"`
	Remark         string            `json:"remark"`
	Listen         string            `json:"listen"`
	Port           int               `json:"port"`
	Protocol       string            `json:"protocol"`
	Enable         bool              `json:"enable"`
	ExpiryTime     int64             `json:"expiryTime"`
	Total          int64             `json:"total"`
	Settings       json.RawMessage   `json:"settings"`
	StreamSettings json.RawMessage   `json:"streamSettings"`
	Sniffing       json.RawMessage   `json:"sniffing"`
	ClientStats    []json.RawMessage `json:"clientStats"`
}

func (s Service) ReconcilePanel(ctx context.Context, p readyworker.Panel) error {
	if s.DB == nil || s.Secrets == nil {
		return errors.New("user capacity config")
	}
	var enabled bool
	var portsRaw []byte
	var target, life, limit, rate int
	var quota int64
	e := s.DB.QueryRowContext(ctx, `SELECT enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second FROM global_config_policies WHERE policy_key='reality'`).Scan(&enabled, &portsRaw, &target, &quota, &life, &limit, &rate)
	if errors.Is(e, sql.ErrNoRows) || !enabled || target <= 0 {
		return nil
	}
	if e != nil {
		return e
	}
	var ports []int
	if json.Unmarshal(portsRaw, &ports) != nil {
		return errors.New("user capacity ports")
	}
	wanted := map[int]bool{}
	for _, v := range ports {
		wanted[v] = true
	}
	var acc, base, user, pref string
	e = s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.base_url,x.username,x.password_secret_ref FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation WHERE pi.id=$1 AND pi.enabled=true`, p.ID).Scan(&acc, &base, &user, &pref)
	if e != nil {
		return e
	}
	pw, e := s.Secrets.Get(ctx, acc, pref)
	if e != nil {
		return e
	}
	client, e := sanaei.NewAPIClient(base, sanaei.Credentials{Username: user, Password: string(pw)}, nil)
	for i := range pw {
		pw[i] = 0
	}
	if e != nil {
		return e
	}
	client.HTTP.Timeout = 8 * time.Second
	if e = client.Login(ctx); e != nil {
		return e
	}
	raws, e := sanaei.ReadRawInboundList(ctx, sanaei.DirectSessionExecutor{Client: client})
	if e != nil {
		return e
	}
	now := time.Now().UnixMilli()
	for _, raw := range raws {
		var in rawInbound
		if json.Unmarshal(raw, &in) != nil || !in.Enable || in.Protocol != "vless" || !wanted[in.Port] {
			continue
		}
		var stream map[string]any
		if json.Unmarshal(in.StreamSettings, &stream) != nil || fmt.Sprint(stream["network"]) != "tcp" || fmt.Sprint(stream["security"]) != "reality" {
			continue
		}
		var st map[string]any
		if json.Unmarshal(in.Settings, &st) != nil {
			continue
		}
		arr, _ := st["clients"].([]any)
		traffic := map[string]sanaei.ClientTraffic{}
		for _, rawStat := range in.ClientStats {
			var stat sanaei.ClientTraffic
			if json.Unmarshal(rawStat, &stat) == nil && stat.Email != "" {
				traffic[stat.Email] = stat
			}
		}
		active, expiredCount, quotaCount := 0, 0, 0
		kept := make([]any, 0, len(arr))
		deleted := 0
		for _, v := range arr {
			b, _ := json.Marshal(v)
			var c sanaei.Client
			if json.Unmarshal(b, &c) != nil || c.ID == "" {
				continue
			}
			stat := traffic[c.Email]
			isExpired := c.ExpiryTime > 0 && c.ExpiryTime <= now
			isQuota := c.TotalGB > 0 && (stat.Up+stat.Down) >= c.TotalGB
			if !c.Enable || isExpired || isQuota {
				deleted++
				if isQuota {
					quotaCount++
				} else {
					expiredCount++
				}
				continue
			}
			active++
			kept = append(kept, v)
		}
		deficit := target - active
		n := 0
		if deficit > 0 {
			n = rate
			if n > deficit {
				n = deficit
			}
		}
		if n > 0 {
			expiry := int64(0)
			if life > 0 {
				expiry = now + int64(life)*1000
			}
			for i := 0; i < n; i++ {
				id, e := sanaei.UUIDv4()
				if e != nil {
					return e
				}
				kept = append(kept, sanaei.Client{ID: id, Email: "dob-" + id[:8], Enable: true, TotalGB: quota, ExpiryTime: expiry, LimitIP: limit, Flow: "xtls-rprx-vision"})
			}
		}
		if n > 0 || deleted > 0 {
			st["clients"] = kept
			var sniff any = map[string]any{"enabled": false}
			if len(in.Sniffing) > 0 {
				_ = json.Unmarshal(in.Sniffing, &sniff)
			}
			payload := map[string]any{"enable": in.Enable, "remark": in.Remark, "listen": in.Listen, "port": in.Port, "protocol": in.Protocol, "expiryTime": in.ExpiryTime, "total": in.Total, "settings": st, "streamSettings": stream, "sniffing": sniff}
			if _, e = sanaei.UpdateInboundRaw(ctx, sanaei.DirectSessionExecutor{Client: client}, int64(in.ID), payload); e != nil {
				return fmt.Errorf("inbound %d update clients: %w", in.ID, e)
			}
		}
		activeAfter := active + n
		deficitAfter := maxInt(target-activeAfter, 0)
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,expired_users,quota_exhausted_users,deficit,created_last_cycle,deleted_last_cycle,last_error,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'',now()) ON CONFLICT(panel_id,inbound_id) DO UPDATE SET port=excluded.port,target_users=excluded.target_users,active_users=excluded.active_users,expired_users=excluded.expired_users,quota_exhausted_users=excluded.quota_exhausted_users,deficit=excluded.deficit,created_last_cycle=excluded.created_last_cycle,deleted_last_cycle=excluded.deleted_last_cycle,last_error='',observed_at=now()`, p.ID, in.ID, in.Port, target, activeAfter, expiredCount, quotaCount, deficitAfter, n, deleted)
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
