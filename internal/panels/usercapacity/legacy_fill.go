package usercapacity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func (s Service) LegacyFastFill(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime, wanted map[int]bool, target int, quota int64, life, limit, rate int) (bool, error) {
	snap, err := sanaei.ReadInventory(ctx, runtime.Session.Exec, p.ID)
	if err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	mutated := false
	for _, rec := range snap.Records {
		if !wanted[rec.Port] || !rec.Enabled || rec.Protocol != "vless" || rec.Transport != "tcp" || rec.Security != "reality" {
			continue
		}
		effectiveTarget, effectiveRate, err := s.effectiveTargetRate(ctx, p.ID, rec.RemoteID, target, rate)
		if err != nil {
			return mutated, err
		}
		ownedPolicies, err := s.activeOwnedPolicy(ctx, p.ID, rec.RemoteID)
		if err != nil {
			return mutated, err
		}
		deficit := effectiveTarget - rec.ClientCount
		n := 0
		if deficit > 0 {
			n = userCreationLimiter.allowance(bulkRateKey(p.ID, rec.RemoteID), effectiveRate, deficit, time.Now())
		}
		if n <= 0 && len(ownedPolicies) == 0 {
			continue
		}
		in, err := sanaei.GetInbound(ctx, runtime.Session.Exec, rec.RemoteID)
		if err != nil {
			return mutated, fmt.Errorf("legacy get inbound %d: %w", rec.RemoteID, err)
		}
		st, err := legacyObject(in.Settings)
		if err != nil {
			return mutated, err
		}
		stream, err := legacyObject(in.StreamSettings)
		if err != nil {
			return mutated, err
		}
		clients, _ := st["clients"].([]any)
		observedOwned := make(map[string]string, len(clients))
		policyExpected := map[string]sanaei.Client{}
		policyChanges := 0
		for i, item := range clients {
			b, _ := json.Marshal(item)
			var existing sanaei.Client
			if json.Unmarshal(b, &existing) == nil && existing.ID != "" {
				observedOwned[existing.ID] = existing.Email
				if own, ok := ownedPolicies[existing.ID]; ok && policyChanges < 32 {
					if desired, changed := desiredOwnedClient(existing, own, quota, life, limit); changed {
						clients[i] = desired
						policyExpected[existing.ID] = desired
						policyChanges++
					}
				}
			}
		}
		if err = s.confirmPlannedOwnedClients(ctx, p.ID, rec.RemoteID, observedOwned); err != nil {
			return mutated, err
		}
		var generation bulkGeneration
		if n > 0 {
			generation, err = s.effectiveGeneration(ctx, p.ID, rec.RemoteID)
			if err != nil {
				return mutated, err
			}
		}
		owned := make([]ownedClient, 0, n)
		expiry := int64(0)
		if life > 0 {
			expiry = now + int64(life)*1000
		}
		for i := 0; i < n; i++ {
			id, e := sanaei.UUIDv4()
			if e != nil {
				return mutated, e
			}
			email := ownershipEmail(generation.Marker, id)
			clients = append(clients, sanaei.Client{ID: id, Email: email, Enable: true, TotalGB: quota, ExpiryTime: expiry, LimitIP: limit, Flow: "xtls-rprx-vision"})
			owned = append(owned, ownedClient{ID: id, Email: email})
		}
		if err = s.planOwnedClients(ctx, generation.ID, owned); err != nil {
			return mutated, err
		}
		st["clients"] = clients
		sniff := in.Sniffing
		payload := map[string]any{"enable": in.Enable, "remark": in.Remark, "port": in.Port, "protocol": in.Protocol, "settings": st, "streamSettings": stream, "sniffing": sniff}
		if _, err = sanaei.UpdateInboundRaw(ctx, runtime.Session.Exec, rec.RemoteID, payload); err != nil {
			return mutated, fmt.Errorf("legacy update inbound %d: %w", rec.RemoteID, err)
		}
		if policyChanges > 0 {
			runtime.Session.Invalidate()
			verifyRaws, ve := runtime.Session.Snapshot(ctx)
			if ve != nil {
				return mutated, fmt.Errorf("legacy inbound %d policy verify snapshot: %w", rec.RemoteID, ve)
			}
			if ve = verifyOwnedPolicySnapshot(verifyRaws, int(rec.RemoteID), policyExpected, nil); ve != nil {
				return mutated, ve
			}
		}
		mutated = true
	}
	if mutated {
		runtime.Session.Invalidate()
	}
	return mutated, nil
}
func legacyObject(v any) (map[string]any, error) {
	switch x := v.(type) {
	case map[string]any:
		return x, nil
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(x), &m) != nil {
			return nil, fmt.Errorf("legacy inbound object")
		}
		return m, nil
	default:
		b, e := json.Marshal(x)
		if e != nil {
			return nil, e
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil, fmt.Errorf("legacy inbound object")
		}
		return m, nil
	}
}

func (s Service) legacyFastFillFromPolicy(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime) (bool, error) {
	var enabled bool
	var portsRaw []byte
	var target, life, limit, rate int
	var quota int64
	err := s.DB.QueryRowContext(ctx, "SELECT enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second FROM global_config_policies WHERE policy_key='reality'").Scan(&enabled, &portsRaw, &target, &quota, &life, &limit, &rate)
	if err != nil || !enabled || target <= 0 {
		return false, err
	}
	var ports []int
	if json.Unmarshal(portsRaw, &ports) != nil {
		return false, fmt.Errorf("legacy capacity ports")
	}
	wanted := map[int]bool{}
	for _, port := range ports {
		wanted[port] = true
	}
	var mutated bool
	err = runtime.WithMutation(ctx, func(runCtx context.Context) error {
		var lifecycle string
		if e := s.DB.QueryRowContext(runCtx, "SELECT r.state FROM panel_instances pi JOIN droplets r ON r.id=pi.droplet_id WHERE pi.id=$1", runtime.PanelID).Scan(&lifecycle); e != nil {
			return e
		}
		if !clientMutationLifecycleAllowed(lifecycle) {
			return nil
		}
		var e error
		mutated, e = s.LegacyFastFill(runCtx, p, runtime, wanted, target, quota, life, limit, rate)
		return e
	})
	return mutated, err
}
