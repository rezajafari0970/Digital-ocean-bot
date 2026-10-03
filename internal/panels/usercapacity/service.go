package usercapacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"sync"
	"time"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}

var addClientUnsupportedPanels sync.Map

func clientMutationLifecycleAllowed(state string) bool {
	return state == "READY" || state == "EXPIRING"
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
	runtime, e := (sanaei.RuntimeFactory{DB: s.DB, Secrets: s.Secrets, Timeout: 8 * time.Second}).Open(ctx, p.ID)
	if e != nil {
		return e
	}
	return s.ReconcileRuntimeFromPolicy(ctx, p, runtime)
}

func (s Service) PolicyPorts(ctx context.Context) ([]int, error) {
	if s.DB == nil {
		return nil, errors.New("user capacity config")
	}
	var enabled bool
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT enabled,ports FROM global_config_policies WHERE policy_key='reality'").Scan(&enabled, &raw)
	if errors.Is(err, sql.ErrNoRows) || !enabled {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ports []int
	if json.Unmarshal(raw, &ports) != nil || len(ports) == 0 {
		return nil, errors.New("user capacity ports")
	}
	return ports, nil
}

func (s Service) FastFillFromPolicy(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime) (bool, error) {
	if s.DB == nil || s.Secrets == nil {
		return false, errors.New("user capacity config")
	}
	allowed, err := s.rolloutAllowed(ctx, p.ID)
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, nil
	}
	if _, unsupported := addClientUnsupportedPanels.Load(p.ID); unsupported {
		return s.legacyFastFillFromPolicy(ctx, p, runtime)
	}
	var enabled bool
	var portsRaw []byte
	var target, life, limit, rate int
	var quota int64
	err = s.DB.QueryRowContext(ctx, "SELECT enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second FROM global_config_policies WHERE policy_key='reality'").Scan(&enabled, &portsRaw, &target, &quota, &life, &limit, &rate)
	if errors.Is(err, sql.ErrNoRows) || !enabled || target <= 0 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var ports []int
	if json.Unmarshal(portsRaw, &ports) != nil {
		return false, errors.New("user capacity ports")
	}
	wanted := map[int]bool{}
	for _, port := range ports {
		wanted[port] = true
	}
	mutated := false
	err = runtime.WithMutation(ctx, func(runCtx context.Context) error {
		var lifecycle string
		if e := s.DB.QueryRowContext(runCtx, "SELECT r.state FROM panel_instances pi JOIN droplets r ON r.id=pi.droplet_id WHERE pi.id=$1", runtime.PanelID).Scan(&lifecycle); e != nil {
			return e
		}
		if !clientMutationLifecycleAllowed(lifecycle) {
			return nil
		}
		var e error
		mutated, e = s.FastFill(runCtx, p, runtime, wanted, target, quota, life, limit, rate)
		return e
	})
	if errors.Is(err, sanaei.ErrAddClientUnsupported) {
		addClientUnsupportedPanels.Store(p.ID, true)
		return false, nil
	}
	return mutated, err
}

func (s Service) ReconcileRuntimeFromPolicy(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime) error {
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
	allowed, e := s.rolloutAllowed(ctx, p.ID)
	if e != nil {
		return e
	}
	if !allowed {
		return nil
	}
	var ports []int
	if json.Unmarshal(portsRaw, &ports) != nil {
		return errors.New("user capacity ports")
	}
	wanted := map[int]bool{}
	for _, v := range ports {
		wanted[v] = true
	}
	return s.ReconcileRuntime(ctx, p, runtime, wanted, target, quota, life, limit, rate)
}

func (s Service) ReconcileRuntime(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime, wanted map[int]bool, target int, quota int64, life, limit, rate int) error {
	if runtime == nil || runtime.Session == nil {
		return errors.New("user capacity runtime")
	}
	return runtime.WithMutation(ctx, func(runCtx context.Context) error {
		return s.reconcileRuntimeLocked(runCtx, p, runtime, wanted, target, quota, life, limit, rate)
	})
}

func (s Service) reconcileRuntimeLocked(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime, wanted map[int]bool, target int, quota int64, life, limit, rate int) error {
	if runtime == nil || runtime.Session == nil {
		return errors.New("user capacity runtime")
	}
	var lifecycle string
	if e := s.DB.QueryRowContext(ctx, "SELECT r.state FROM panel_instances pi JOIN droplets r ON r.id=pi.droplet_id WHERE pi.id=$1", runtime.PanelID).Scan(&lifecycle); e != nil {
		return e
	}
	if !clientMutationLifecycleAllowed(lifecycle) {
		return nil
	}
	runtime.Session.Invalidate()
	raws, e := runtime.Session.Snapshot(ctx)
	if e != nil {
		return e
	}
	now := time.Now().UnixMilli()
	mutated := false
	_, addUnsupported := addClientUnsupportedPanels.Load(p.ID)
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
		observedOwned := make(map[string]string, len(arr))
		kept := make([]any, 0, len(arr))
		deleteIDs := make([]string, 0)
		deleted := 0
		for _, v := range arr {
			b, _ := json.Marshal(v)
			var c sanaei.Client
			if json.Unmarshal(b, &c) != nil || c.ID == "" {
				continue
			}
			observedOwned[c.ID] = c.Email
			stat := traffic[c.Email]
			isExpired := c.ExpiryTime > 0 && c.ExpiryTime <= now
			isQuota := c.TotalGB > 0 && (stat.Up+stat.Down) >= c.TotalGB
			if !c.Enable || isExpired || isQuota {
				deleted++
				deleteIDs = append(deleteIDs, c.ID)
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
		if e = s.confirmPlannedOwnedClients(ctx, p.ID, int64(in.ID), observedOwned); e != nil {
			return e
		}
		deficit := target - active
		n := userCreationLimiter.allowance(bulkRateKey(p.ID, int64(in.ID)), rate, deficit, time.Now())
		newClients := make([]sanaei.Client, 0, n)
		owned := make([]ownedClient, 0, n)
		var generation bulkGeneration
		if n > 0 {
			generation, e = s.activePolicyGeneration(ctx, p.ID, int64(in.ID))
			if e != nil {
				return e
			}
			expiry := int64(0)
			if life > 0 {
				expiry = now + int64(life)*1000
			}
			for i := 0; i < n; i++ {
				id, e := sanaei.UUIDv4()
				if e != nil {
					return e
				}
				email := ownershipEmail(generation.Marker, id)
				c := sanaei.Client{ID: id, Email: email, Enable: true, TotalGB: quota, ExpiryTime: expiry, LimitIP: limit, Flow: "xtls-rprx-vision"}
				newClients = append(newClients, c)
				owned = append(owned, ownedClient{ID: id, Email: email})
				kept = append(kept, c)
			}
		}
		if len(owned) > 0 {
			if e = s.planOwnedClients(ctx, generation.ID, owned); e != nil {
				return e
			}
		}
		if !addUnsupported && deleted > 0 && deleted <= 32 {
			for _, clientID := range deleteIDs {
				if e = sanaei.DeleteClientSession(ctx, runtime.Session.Exec, in.ID, clientID); e != nil {
					return fmt.Errorf("inbound %d delete client %s: %w", in.ID, clientID, e)
				}
			}
			if len(newClients) > 0 {
				if e = sanaei.AddClientsSession(ctx, runtime.Session.Exec, in.ID, newClients); e != nil {
					return fmt.Errorf("inbound %d add clients: %w", in.ID, e)
				}
			}
			mutated = true
		} else if !addUnsupported && deleted == 0 && len(newClients) > 0 {
			if e = sanaei.AddClientsSession(ctx, runtime.Session.Exec, in.ID, newClients); e != nil {
				return fmt.Errorf("inbound %d add clients: %w", in.ID, e)
			}
			mutated = true
		} else if n > 0 || deleted > 0 {
			st["clients"] = kept
			var sniff any = map[string]any{"enabled": false}
			if len(in.Sniffing) > 0 {
				_ = json.Unmarshal(in.Sniffing, &sniff)
			}
			payload := map[string]any{"enable": in.Enable, "remark": in.Remark, "listen": in.Listen, "port": in.Port, "protocol": in.Protocol, "expiryTime": in.ExpiryTime, "total": in.Total, "settings": st, "streamSettings": stream, "sniffing": sniff}
			if _, e = sanaei.UpdateInboundRaw(ctx, runtime.Session.Exec, int64(in.ID), payload); e != nil {
				return fmt.Errorf("inbound %d update clients: %w", in.ID, e)
			}
			mutated = true
		}
		activeAfter := active + n
		deficitAfter := maxInt(target-activeAfter, 0)
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,expired_users,quota_exhausted_users,deficit,created_last_cycle,deleted_last_cycle,last_error,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'',now()) ON CONFLICT(panel_id,inbound_id) DO UPDATE SET port=excluded.port,target_users=excluded.target_users,active_users=excluded.active_users,expired_users=excluded.expired_users,quota_exhausted_users=excluded.quota_exhausted_users,deficit=excluded.deficit,created_last_cycle=excluded.created_last_cycle,deleted_last_cycle=excluded.deleted_last_cycle,last_error='',observed_at=now()`, p.ID, in.ID, in.Port, target, activeAfter, expiredCount, quotaCount, deficitAfter, n, deleted)
	}
	if mutated {
		runtime.Session.Invalidate()
	}
	return nil
}

func (s Service) NeedsReconcile(ctx context.Context, panelID string, ports []int, cooldown time.Duration) (bool, error) {
	if s.DB == nil || panelID == "" {
		return true, nil
	}
	if cooldown <= 0 {
		cooldown = 45 * time.Second
	}
	var count, deficient int
	var newest sql.NullTime
	err := s.DB.QueryRowContext(ctx, `
SELECT count(*), count(*) FILTER (WHERE deficit > 0), max(observed_at)
FROM user_capacity_snapshots
WHERE panel_id=$1 AND port = ANY($2)
`, panelID, pq.Array(ports)).Scan(&count, &deficient, &newest)
	if err != nil {
		return true, err
	}
	if count < len(ports) || deficient > 0 || !newest.Valid {
		return true, nil
	}
	return time.Since(newest.Time) >= cooldown, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s Service) rolloutAllowed(ctx context.Context, panelID string) (bool, error) {
	var mode string
	err := s.DB.QueryRowContext(ctx, `SELECT mode FROM reality_rollout_control WHERE policy_key='reality'`).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if mode == "stable" {
		return true, nil
	}
	var allowed bool
	if err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reality_rollout_panels WHERE panel_id=$1 AND enabled=true)`, panelID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}
