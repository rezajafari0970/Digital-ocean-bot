package clientops

import (
	"encoding/json"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type ClientPatch struct {
	Email      *string `json:"email,omitempty"`
	Enable     *bool   `json:"enable,omitempty"`
	TotalGB    *int64  `json:"totalGB,omitempty"`
	ExpiryTime *int64  `json:"expiryTime,omitempty"`
	LimitIP    *int    `json:"limitIp,omitempty"`
	LimitHWID  *int    `json:"limitHwid,omitempty"`
	Flow       *string `json:"flow,omitempty"`
}

func (p ClientPatch) Empty() bool {
	return p.Email == nil && p.Enable == nil && p.TotalGB == nil &&
		p.ExpiryTime == nil && p.LimitIP == nil && p.LimitHWID == nil && p.Flow == nil
}

func payloadPatch(raw json.RawMessage) (ClientPatch, error) {
	var envelope struct {
		Patch ClientPatch `json:"Patch"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ClientPatch{}, err
	}
	if envelope.Patch.Empty() {
		return ClientPatch{}, ErrInvalidRequest
	}
	return envelope.Patch, nil
}

func applyClientPatch(c sanaei.Client, p ClientPatch) sanaei.Client {
	if p.Email != nil {
		c.Email = *p.Email
	}
	if p.Enable != nil {
		c.Enable = *p.Enable
	}
	if p.TotalGB != nil {
		c.TotalGB = *p.TotalGB
	}
	if p.ExpiryTime != nil {
		c.ExpiryTime = *p.ExpiryTime
	}
	if p.LimitIP != nil {
		c.LimitIP = *p.LimitIP
	}
	if p.Flow != nil {
		c.Flow = *p.Flow
	}
	return c
}

func patchSatisfied(c sanaei.Client, p ClientPatch) bool {
	if p.Email != nil && c.Email != *p.Email {
		return false
	}
	if p.Enable != nil && c.Enable != *p.Enable {
		return false
	}
	if p.TotalGB != nil && c.TotalGB != *p.TotalGB {
		return false
	}
	if p.ExpiryTime != nil && c.ExpiryTime != *p.ExpiryTime {
		return false
	}
	if p.LimitIP != nil && c.LimitIP != *p.LimitIP {
		return false
	}
	if p.Flow != nil && c.Flow != *p.Flow {
		return false
	}
	return true
}

func patchInboundClient(raw json.RawMessage, clientID string, patch ClientPatch) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	settingsRaw, ok := doc["settings"]
	if !ok {
		return nil, ErrInboundMissing
	}
	var settings map[string]any
	switch v := settingsRaw.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &settings); err != nil {
			return nil, err
		}
	case map[string]any:
		settings = v
	default:
		return nil, ErrInboundMissing
	}

	clients, ok := settings["clients"].([]any)
	if !ok {
		return nil, ErrClientConflict
	}
	found := false
	for i, item := range clients {
		b, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		var c sanaei.Client
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		if c.ID != clientID {
			continue
		}
		found = true
		clients[i] = applyClientPatch(c, patch)
	}
	if !found {
		return nil, ErrClientConflict
	}
	settings["clients"] = clients
	doc["settings"] = settings
	return doc, nil
}

func clientMapFromInbound(raw json.RawMessage, clientID string) (map[string]any, bool, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, err
	}
	sr, ok := doc["settings"]
	if !ok {
		return nil, false, ErrInboundMissing
	}
	var settings map[string]any
	switch v := sr.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &settings); err != nil {
			return nil, false, err
		}
	case map[string]any:
		settings = v
	default:
		return nil, false, ErrInboundMissing
	}
	items, ok := settings["clients"].([]any)
	if !ok {
		return nil, false, ErrClientConflict
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id == clientID {
			return m, true, nil
		}
	}
	return nil, false, nil
}
func applyPatchMap(m map[string]any, p ClientPatch) (map[string]any, error) {
	if p.Email != nil {
		return nil, ErrUnsupportedKind
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	if p.Enable != nil {
		out["enable"] = *p.Enable
	}
	if p.TotalGB != nil {
		out["totalGB"] = *p.TotalGB
	}
	if p.ExpiryTime != nil {
		out["expiryTime"] = *p.ExpiryTime
	}
	if p.LimitIP != nil {
		out["limitIp"] = *p.LimitIP
	}
	if p.LimitHWID != nil {
		out["limitHwid"] = *p.LimitHWID
	}
	if p.Flow != nil {
		out["flow"] = *p.Flow
	}
	return out, nil
}
func mapPatchSatisfied(m map[string]any, p ClientPatch) bool {
	if p.Email != nil {
		return false
	}
	eqNum := func(k string, w int64) bool { v, ok := m[k].(float64); return ok && int64(v) == w }
	if p.Enable != nil {
		v, ok := m["enable"].(bool)
		if !ok || v != *p.Enable {
			return false
		}
	}
	if p.TotalGB != nil && !eqNum("totalGB", *p.TotalGB) {
		return false
	}
	if p.ExpiryTime != nil && !eqNum("expiryTime", *p.ExpiryTime) {
		return false
	}
	if p.LimitIP != nil && !eqNum("limitIp", int64(*p.LimitIP)) {
		return false
	}
	if p.LimitHWID != nil && !eqNum("limitHwid", int64(*p.LimitHWID)) {
		return false
	}
	if p.Flow != nil {
		v, ok := m["flow"].(string)
		if !ok || v != *p.Flow {
			return false
		}
	}
	return true
}

func v3UpdatePayload(global map[string]any, p ClientPatch) (map[string]any, error) {
	if p.Email != nil {
		return nil, ErrUnsupportedKind
	}
	out := map[string]any{}
	copyKeys := []string{"email", "subId", "password", "auth", "flow", "security", "totalGB", "expiryTime", "reset", "resetDay", "resetWeekday", "resetMax", "trafficReset", "trafficResetDay", "limitIp", "limitHwid", "tgId", "group", "comment", "enable", "privateKey", "publicKey", "preSharedKey", "keepAlive", "forwardedPorts", "secret", "adTag", "reverse"}
	for _, k := range copyKeys {
		if v, ok := global[k]; ok && v != nil {
			out[k] = v
		}
	}
	if uuid, _ := global["uuid"].(string); uuid != "" {
		out["id"] = uuid
		out["uuid"] = uuid
	}
	if raw, ok := global["allowedIPs"].(string); ok && strings.TrimSpace(raw) != "" {
		parts := []string{}
		for _, x := range strings.Split(raw, ",") {
			if x = strings.TrimSpace(x); x != "" {
				parts = append(parts, x)
			}
		}
		if len(parts) > 0 {
			out["allowedIPs"] = parts
		}
	}
	return applyPatchMap(out, p)
}
