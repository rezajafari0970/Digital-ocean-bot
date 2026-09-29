package usercapacity

import (
	"context"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func (s Service) FastFill(ctx context.Context, p readyworker.Panel, runtime *sanaei.PanelRuntime, wanted map[int]bool, target int, quota int64, life, limit, rate int) (bool, error) {
	if runtime == nil || runtime.Session == nil || target <= 0 || rate <= 0 {
		return false, nil
	}
	snap, err := sanaei.ReadInventory(ctx, runtime.Session.Exec, p.ID)
	if err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	mutated := false
	for _, in := range snap.Records {
		if !wanted[in.Port] || !in.Enabled || in.Protocol != "vless" || in.Transport != "tcp" || in.Security != "reality" {
			continue
		}
		deficit := target - in.ClientCount
		if deficit <= 0 {
			continue
		}
		n := rate
		if n > deficit {
			n = deficit
		}
		expiry := int64(0)
		if life > 0 {
			expiry = now + int64(life)*1000
		}
		clients := make([]sanaei.Client, 0, n)
		for i := 0; i < n; i++ {
			id, e := sanaei.UUIDv4()
			if e != nil {
				return mutated, e
			}
			clients = append(clients, sanaei.Client{ID: id, Email: "dob-" + id[:8], Enable: true, TotalGB: quota, ExpiryTime: expiry, LimitIP: limit, Flow: "xtls-rprx-vision"})
		}
		if err = sanaei.AddClientsSession(ctx, runtime.Session.Exec, int(in.RemoteID), clients); err != nil {
			return mutated, fmt.Errorf("inbound %d fast fill: %w", in.RemoteID, err)
		}
		mutated = true
	}
	if mutated {
		runtime.Session.Invalidate()
	}
	return mutated, nil
}
