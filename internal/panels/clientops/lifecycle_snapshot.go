package clientops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

type LifecycleClient struct {
	Client  sanaei.Client
	Traffic *sanaei.ClientTraffic
}

func (c LifecycleClient) InactiveReason(now time.Time) (string, error) {
	if !c.Client.Enable {
		return "DISABLED", nil
	}
	if c.Client.ExpiryTime > 0 && c.Client.ExpiryTime <= now.UnixMilli() {
		return "EXPIRED", nil
	}
	if c.Client.TotalGB > 0 {
		if c.Traffic == nil || c.Traffic.Email != c.Client.Email || c.Traffic.Up < 0 || c.Traffic.Down < 0 {
			return "", ErrVerify
		}
		// Subtraction avoids overflowing summed traffic counters.
		if c.Traffic.Up >= c.Client.TotalGB || c.Traffic.Down >= c.Client.TotalGB-c.Traffic.Up {
			return "QUOTA", nil
		}
	}
	return "", nil
}

// One fresh inbound/global pair supplies identity, policy and global traffic.
// Missing global records, duplicate identities and cross-inbound sharing fail closed.
var errLifecyclePolicyDisagreement = fmt.Errorf("%w: inbound/global policy disagreement", ErrVerify)

// The v3 client record and inbound projection can change between our two reads
// (for example when the panel disables an expired client). Re-read both views;
// never merge conflicting values or retry any mutation. Persistent disagreement
// still returns ErrVerify and retains the worker's fail-closed behavior.
func LifecycleInventory(ctx context.Context, rt *sanaei.PanelRuntime, inbound int64) (map[string]LifecycleClient, int, error) {
	for attempt := 0; ; attempt++ {
		clients, port, err := lifecycleInventoryOnce(ctx, rt, inbound)
		if !errors.Is(err, errLifecyclePolicyDisagreement) || attempt >= 2 {
			return clients, port, err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, 0, fmt.Errorf("%w: %w", sanaei.ErrInventoryRejected, ctx.Err())
		case <-timer.C:
		}
	}
}

func lifecycleInventoryOnce(ctx context.Context, rt *sanaei.PanelRuntime, inbound int64) (map[string]LifecycleClient, int, error) {
	rt.Session.Invalidate()
	raws, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return nil, 0, err
	}
	var clients []sanaei.Client
	port := 0
	found := false
	for _, raw := range raws {
		var in struct {
			ID       int64           `json:"id"`
			Port     int             `json:"port"`
			Enable   bool            `json:"enable"`
			Protocol string          `json:"protocol"`
			Settings json.RawMessage `json:"settings"`
			Stream   json.RawMessage `json:"streamSettings"`
		}
		if err = json.Unmarshal(raw, &in); err != nil {
			return nil, 0, fmt.Errorf("%w: %w", sanaei.ErrInventoryRejected, err)
		}
		if in.ID != inbound {
			continue
		}
		if found || !in.Enable || in.Protocol != "vless" {
			return nil, 0, ErrInboundMissing
		}
		found = true
		port = in.Port
		var stream struct {
			Network  string `json:"network"`
			Security string `json:"security"`
		}
		if json.Unmarshal(in.Stream, &stream) != nil || stream.Network != "tcp" || stream.Security != "reality" {
			return nil, 0, ErrClientConflict
		}
		b := in.Settings
		var encoded string
		if json.Unmarshal(b, &encoded) == nil {
			b = []byte(encoded)
		}
		var settings struct {
			Clients *[]sanaei.Client `json:"clients"`
		}
		if json.Unmarshal(b, &settings) != nil || settings.Clients == nil {
			return nil, 0, ErrVerify
		}
		clients = *settings.Clients
	}
	if !found {
		return nil, 0, ErrInboundMissing
	}
	globals, err := globalWanted(ctx, rt, inbound, clients)
	if err != nil {
		return nil, 0, err
	}
	out := map[string]LifecycleClient{}
	emails := map[string]bool{}
	for _, c := range clients {
		g, ok := globals[c.ID]
		if !ok || c.ID == "" || c.Email == "" || emails[c.Email] {
			return nil, 0, ErrClientConflict
		}
		if _, ok = out[c.ID]; ok {
			return nil, 0, ErrClientConflict
		}
		emails[c.Email] = true
		if c.Email != g.Email || c.Enable != g.Enable || c.TotalGB != g.TotalGB || c.ExpiryTime != g.ExpiryTime {
			return nil, 0, errLifecyclePolicyDisagreement
		}
		c.LimitHWID = g.LimitHWID
		out[c.ID] = LifecycleClient{c, g.Traffic}
	}
	return out, port, nil
}
