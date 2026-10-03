package clientops

import (
	"encoding/json"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

type ClientPatch struct {
	Email      *string `json:"email,omitempty"`
	Enable     *bool   `json:"enable,omitempty"`
	TotalGB    *int64  `json:"totalGB,omitempty"`
	ExpiryTime *int64  `json:"expiryTime,omitempty"`
	LimitIP    *int    `json:"limitIp,omitempty"`
	Flow       *string `json:"flow,omitempty"`
}

func (p ClientPatch) Empty() bool {
	return p.Email == nil && p.Enable == nil && p.TotalGB == nil &&
		p.ExpiryTime == nil && p.LimitIP == nil && p.Flow == nil
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
