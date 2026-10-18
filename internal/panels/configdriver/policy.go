package configdriver

import (
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"time"
)

// ClassSettings belongs to one profile and one routing class. Zero lifetime,
// quota, device limit and creation interval explicitly mean unlimited/automatic.
type ClassSettings struct {
	Count                   int    `json:"count"`
	LifetimeSeconds         int    `json:"lifetime_seconds"`
	DeviceLimit             int    `json:"device_limit"`
	QuotaBytes              int64  `json:"quota_bytes"`
	UsersPerSecond          int    `json:"users_per_second"`
	CreationIntervalSeconds int    `json:"creation_interval_seconds"`
	Flow                    string `json:"flow"`
	Publish                 bool   `json:"publish"`
	AllowInsecure           bool   `json:"allow_insecure"`
	FinalMask               string `json:"final_mask,omitempty"`
	ExportFinalMask         bool   `json:"export_final_mask"`
}

func (p ClassSettings) Validate(s Spec) error {
	if p.Count < 1 || p.Count > 100 || p.LifetimeSeconds < 0 || p.LifetimeSeconds > 31536000 || p.DeviceLimit < 0 || p.DeviceLimit > 1000 || p.QuotaBytes < 0 || p.UsersPerSecond < 0 || p.UsersPerSecond > 100 || p.CreationIntervalSeconds < 0 || p.CreationIntervalSeconds > 86400 {
		return errors.New("profile_class_settings_invalid")
	}
	if p.Flow != "" && (s.Protocol != "vless" || s.Network != "tcp" || s.Security != "reality" || p.Flow != "xtls-rprx-vision") {
		return errors.New("profile_flow_unsupported")
	}
	if p.AllowInsecure && s.Security != "tls" {
		return errors.New("profile_allow_insecure_requires_tls")
	}
	if p.FinalMask != "" {
		if s.Protocol == "shadowsocks" || s.Security == "none" {
			return errors.New("profile_final_mask_requires_tls_or_reality")
		}
		c := transportprofile.Config{FinalMask: json.RawMessage(p.FinalMask)}
		if _, e := c.ClientTCPMask(); e != nil {
			return errors.New("profile_final_mask_invalid")
		}
	}
	if p.ExportFinalMask && p.FinalMask == "" {
		return errors.New("profile_final_mask_required")
	}
	return nil
}

func ClientFlow(s Spec, c Client) string {
	if c.PolicySet {
		return c.Flow
	}
	return s.Flow
}
func Active(c Client, now time.Time) bool { return c.ExpiresAt == 0 || c.ExpiresAt > now.UnixMilli() }

// ResizePolicies is deterministic for retained users. Edits never extend their
// creation time or reset another class's expiry/rate budget.
func ResizePolicies(previous Credentials, profile, panel string, classes []string, settings map[string]ClassSettings, now time.Time, fallbackFlow ...string) (Credentials, error) {
	if len(classes) < 1 || len(classes) > 2 {
		return Credentials{}, errors.New("profile_classes_invalid")
	}
	seen := map[string]bool{}
	for _, cl := range classes {
		if seen[cl] || (cl != "DIRECT" && cl != "RESIDENTIAL") {
			return Credentials{}, errors.New("profile_classes_invalid")
		}
		seen[cl] = true
	}
	// Preserve adopted unlimited credentials byte for byte until an explicit
	// client policy actually changes their behavior.
	legacy := true
	inheritedFlow := ""
	if len(fallbackFlow) > 0 {
		inheritedFlow = fallbackFlow[0]
	}
	for _, cl := range classes {
		policy, ok := settings[cl]
		if !ok || policy.Count < 1 || policy.Count > 100 {
			return Credentials{}, errors.New("profile_class_settings_invalid")
		}
		count := 0
		for _, c := range previous.Clients {
			if c.Class == cl {
				count++
				if c.PolicySet {
					legacy = false
				}
			}
		}
		if count != policy.Count || policy.LifetimeSeconds != 0 || policy.DeviceLimit != 0 || policy.QuotaBytes != 0 || policy.UsersPerSecond != 0 || policy.CreationIntervalSeconds != 0 || policy.Flow != inheritedFlow || !policy.Publish || policy.AllowInsecure || policy.ExportFinalMask {
			legacy = false
		}
	}
	if legacy && len(previous.Clients) > 0 {
		for _, c := range previous.Clients {
			if !seen[c.Class] {
				legacy = false
			}
		}
		if legacy {
			return previous, nil
		}
	}
	out := previous
	out.Clients = nil
	out.LastCreated = map[string]int64{}
	for k, v := range previous.LastCreated {
		out.LastCreated[k] = v
	}
	for _, class := range classes {
		policy, ok := settings[class]
		if !ok {
			return out, errors.New("profile_class_settings_missing")
		}
		members := []Client{}
		last := out.LastCreated[class]
		for _, c := range previous.Clients {
			if c.Class != class {
				continue
			}
			if c.CreatedAt > last {
				last = c.CreatedAt
			}
			if c.CreatedAt == 0 {
				c.CreatedAt = now.UnixMilli()
			}
			if c.CreatedAt > last {
				last = c.CreatedAt
			}
			c.ExpiresAt = 0
			if policy.LifetimeSeconds > 0 {
				c.ExpiresAt = c.CreatedAt + int64(policy.LifetimeSeconds)*1000
			}
			if Active(c, now) && len(members) < policy.Count {
				members = append(members, c)
			}
		}
		missing := policy.Count - len(members)
		interval := int64(policy.CreationIntervalSeconds) * 1000
		if policy.UsersPerSecond > 0 && interval < 1000 {
			interval = 1000
		}
		if missing > 0 && (last == 0 || now.UnixMilli()-last >= interval) {
			batch := missing
			if policy.UsersPerSecond > 0 && batch > policy.UsersPerSecond {
				batch = policy.UsersPerSecond
			}
			added, e := NewCredentials(profile, panel, []string{class}, batch)
			if e != nil {
				return out, e
			}
			out.LastCreated[class] = now.UnixMilli()
			for _, c := range added.Clients {
				c.CreatedAt = now.UnixMilli()
				if policy.LifetimeSeconds > 0 {
					c.ExpiresAt = c.CreatedAt + int64(policy.LifetimeSeconds)*1000
				}
				members = append(members, c)
			}
		}
		if last > out.LastCreated[class] {
			out.LastCreated[class] = last
		}
		for _, c := range members {
			c.PolicySet = true
			c.Flow = policy.Flow
			c.DeviceLimit = policy.DeviceLimit
			c.QuotaBytes = policy.QuotaBytes
			c.AllowInsecure = policy.AllowInsecure
			c.FinalMask = ""
			if policy.ExportFinalMask {
				c.FinalMask = policy.FinalMask
			}
			c.Publish = policy.Publish
			out.Clients = append(out.Clients, c)
		}
	}
	return out, nil
}
