package configplane

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"strings"
)

func managedClientPolicies(c configdriver.Credentials) bool {
	for _, cl := range c.Clients {
		if cl.PolicySet {
			return true
		}
	}
	return false
}
func globalPolicyMatches(cl configdriver.Client, live sanaei.GlobalClient) bool {
	return live.UUID == cl.ID && live.Email == cl.Email && live.ExpiryTime == cl.ExpiresAt && live.TotalGB == cl.QuotaBytes && live.LimitHWID == cl.DeviceLimit && live.Flow == cl.Flow
}
func ownedGlobalPolicies(c configdriver.Credentials, users []sanaei.GlobalClient) (map[string]sanaei.GlobalClient, error) {
	owned := map[string]bool{}
	for _, cl := range c.Clients {
		if cl.PolicySet {
			if owned[cl.Email] {
				return nil, errors.New("profile_global_policy_ambiguous")
			}
			owned[cl.Email] = true
		}
	}
	byEmail := map[string]sanaei.GlobalClient{}
	for _, u := range users {
		if !owned[u.Email] {
			continue
		}
		if _, ok := byEmail[u.Email]; ok {
			return nil, errors.New("profile_global_policy_ambiguous")
		}
		byEmail[u.Email] = u
	}
	return byEmail, nil
}
func verifyGlobalPolicies(c configdriver.Credentials, users []sanaei.GlobalClient) error {
	byEmail, e := ownedGlobalPolicies(c, users)
	if e != nil {
		return e
	}
	for _, cl := range c.Clients {
		if cl.PolicySet && !globalPolicyMatches(cl, byEmail[cl.Email]) {
			return errors.New("profile_global_policy_unverified")
		}
	}
	return nil
}

// In Sanaei v3 device limits belong to the global client record. The inbound
// endpoint ignores limitHwid. Read the exact owned UUID and preserve all native
// update fields, then apply only the requested policy and verify by fresh read.
func (s Service) applyGlobalPolicies(ctx context.Context, panel string, exec sanaei.SessionExecutor, c configdriver.Credentials) error {
	if !managedClientPolicies(c) {
		return nil
	}
	users, e := sanaei.ReadGlobalClientsSession(ctx, exec)
	if e != nil {
		return errors.New("profile_global_policy_inventory_unavailable")
	}
	byEmail, e := ownedGlobalPolicies(c, users)
	if e != nil {
		return e
	}
	changed := false
	for _, cl := range c.Clients {
		if !cl.PolicySet || globalPolicyMatches(cl, byEmail[cl.Email]) {
			continue
		}
		live, ok := byEmail[cl.Email]
		if !ok || live.UUID != cl.ID {
			return errors.New("profile_global_policy_identity_conflict")
		}
		if !sanaei.ConfigLockHeld(ctx, s.DB, panel) {
			return sanaei.ErrConfigOwnership
		}
		current, e := sanaei.GetClientByEmailSession(ctx, exec, cl.Email)
		if e != nil {
			return errors.New("profile_global_policy_read_failed")
		}
		if current["uuid"] != cl.ID || current["email"] != cl.Email {
			return errors.New("profile_global_policy_identity_conflict")
		}
		payload := globalPolicyPayload(current, cl)
		// A timeout can acknowledge a successful SET. Always observe before retry.
		_ = sanaei.UpdateClientByEmailSession(ctx, exec, cl.Email, payload)
		changed = true
	}
	if changed {
		users, e = sanaei.ReadGlobalClientsSession(ctx, exec)
		if e != nil {
			return errors.New("profile_global_policy_inventory_unavailable")
		}
	}
	return verifyGlobalPolicies(c, users)
}
func globalPolicyPayload(current map[string]any, cl configdriver.Client) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"email", "subId", "password", "auth", "flow", "security", "totalGB", "expiryTime", "reset", "resetDay", "resetWeekday", "resetMax", "trafficReset", "trafficResetDay", "limitIp", "limitHwid", "tgId", "group", "comment", "enable", "privateKey", "publicKey", "preSharedKey", "keepAlive", "forwardedPorts", "secret", "adTag", "reverse"} {
		if v, ok := current[key]; ok && v != nil {
			out[key] = v
		}
	}
	if raw, ok := current["allowedIPs"].([]any); ok {
		out["allowedIPs"] = append([]any(nil), raw...)
	}
	if raw, ok := current["allowedIPs"].([]string); ok {
		out["allowedIPs"] = append([]string(nil), raw...)
	}
	out["id"] = cl.ID
	out["uuid"] = cl.ID
	if raw, ok := current["allowedIPs"].(string); ok && strings.TrimSpace(raw) != "" {
		ips := []string{}
		for _, ip := range strings.Split(raw, ",") {
			if ip = strings.TrimSpace(ip); ip != "" {
				ips = append(ips, ip)
			}
		}
		out["allowedIPs"] = ips
	}
	out["expiryTime"] = cl.ExpiresAt
	out["totalGB"] = cl.QuotaBytes
	out["limitHwid"] = cl.DeviceLimit
	out["flow"] = cl.Flow
	return out
}

// A v3 inbound update preserves the existing global Flow. Complete its explicit
// policy SET only after a durable SENT journal and exact membership observation.
// Full observation after fresh read remains mandatory for native confirmation.
func (s Service) observeJournaledWrite(ctx context.Context, panel string, rt *sanaei.PanelRuntime, raws []json.RawMessage, payload map[string]any, remote int64, credentials configdriver.Credentials) ([]json.RawMessage, int64, error) {
	id, e := configdriver.Observe(raws, payload, remote, credentials)
	if e == nil && id > 0 {
		return raws, id, nil
	}
	id, e = configdriver.ObservePendingFlow(raws, payload, remote, credentials)
	if e != nil || id == 0 {
		return raws, 0, errors.New("profile_write_outcome_unconfirmed")
	}
	if e = s.applyGlobalPolicies(ctx, panel, rt.Session.Exec, credentials); e != nil {
		return raws, 0, e
	}
	rt.Session.Invalidate()
	raws, e = rt.Session.FreshSnapshot(ctx)
	if e != nil {
		return raws, 0, errors.New("profile_write_outcome_unconfirmed")
	}
	id, e = configdriver.Observe(raws, payload, remote, credentials)
	if e != nil || id == 0 {
		return raws, 0, errors.New("profile_write_outcome_unconfirmed")
	}
	return raws, id, nil
}
