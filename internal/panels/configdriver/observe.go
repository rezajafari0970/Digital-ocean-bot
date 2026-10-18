package configdriver

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"
)

func Object(raw json.RawMessage) (map[string]any, error) {
	var nested string
	if json.Unmarshal(raw, &nested) == nil {
		raw = []byte(nested)
	}
	var v map[string]any
	e := json.Unmarshal(raw, &v)
	if v == nil && e == nil {
		e = errors.New("profile_invalid_object")
	}
	return v, e
}
func normalized(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}
func subset(got, want any) bool {
	m, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(got, want)
	}
	g, ok := got.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range m {
		if !subset(g[k], v) {
			return false
		}
	}
	return true
}

// Observe requires the durable ownership name, optional native ID, exact client
// membership and desired transport. It never adopts an arbitrary matching port.
func Observe(raws []json.RawMessage, desired map[string]any, remote int64, policies ...Credentials) (int64, error) {
	return observe(raws, desired, remote, false, policies...)
}

// ObservePendingFlow checks an already-journaled native write before its separate
// global-client Flow SET. Only owned policy-managed Flow may differ; identity,
// membership, transport and all other limits retain strict checks.
func ObservePendingFlow(raws []json.RawMessage, desired map[string]any, remote int64, policies Credentials) (int64, error) {
	return observe(raws, desired, remote, true, policies)
}
func observe(raws []json.RawMessage, desired map[string]any, remote int64, pendingFlow bool, policies ...Credentials) (int64, error) {
	want := normalized(desired).(map[string]any)
	globalPolicy := map[string]bool{}
	for _, creds := range policies {
		for _, cl := range creds.Clients {
			if cl.PolicySet {
				globalPolicy[cl.ID] = true
			}
		}
	}
	id := int64(0)
	for _, raw := range raws {
		in, e := Object(raw)
		if e != nil {
			return 0, errors.New("profile_inventory_invalid")
		}
		rid, ok := in["id"].(float64)
		if !ok || rid < 1 {
			return 0, errors.New("profile_inventory_invalid")
		}
		if in["remark"] != want["remark"] {
			if in["port"] == want["port"] {
				return 0, errors.New("profile_port_occupied")
			}
			if remote > 0 && int64(rid) == remote {
				return 0, errors.New("profile_listener_identity_changed")
			}
			continue
		}
		if id != 0 || remote > 0 && int64(rid) != remote {
			return 0, errors.New("profile_listener_identity_changed")
		}
		for _, k := range []string{"protocol", "port", "enable", "total", "expiryTime"} {
			// Panels omit zero-valued limits in slim inventory.
			actual := in[k]
			if actual == nil && (k == "total" || k == "expiryTime") {
				actual = float64(0)
			}
			if !reflect.DeepEqual(actual, want[k]) {
				return 0, errors.New("profile_listener_mismatch")
			}
		}
		if in["listen"] != "" && in["listen"] != "0.0.0.0" {
			return 0, errors.New("profile_listener_mismatch")
		}
		for _, k := range []string{"settings", "streamSettings", "sniffing"} {
			b, _ := json.Marshal(in[k])
			m, e := Object(b)
			if e != nil {
				return 0, errors.New("profile_native_config_invalid")
			}
			expected := want[k].(map[string]any)
			if k == "settings" {
				gotClients, ok := m["clients"].([]any)
				wanted := expected["clients"].([]any)
				if !ok || len(gotClients) != len(wanted) {
					return 0, errors.New("profile_client_membership_changed")
				}
				byID := map[string]any{}
				for _, cl := range gotClients {
					c, ok := cl.(map[string]any)
					if !ok {
						return 0, errors.New("profile_client_invalid")
					}
					key, _ := c["id"].(string)
					if key == "" || byID[key] != nil {
						return 0, errors.New("profile_client_invalid")
					}
					byID[key] = c
				}
				for _, cl := range wanted {
					c := cl.(map[string]any)
					g, ok := byID[c["id"].(string)].(map[string]any)
					if ok && want["protocol"] == "shadowsocks" {
						if method, exists := g["security"]; exists {
							g["method"] = method
						}
					}
					if ok {
						if expiry, ok := c["expiryTime"].(float64); ok && expiry > 0 && expiry <= float64(time.Now().UnixMilli()) && g["enable"] == false {
							c["enable"] = false
						}
					}
					if ok {
						for _, k := range []string{"totalGB", "expiryTime", "limitHwid"} {
							if g[k] == nil && c[k] == float64(0) {
								g[k] = float64(0)
							}
						}
					}
					if globalPolicy[c["id"].(string)] {
						delete(c, "limitHwid")
						delete(g, "limitHwid")
					}
					if pendingFlow && globalPolicy[c["id"].(string)] {
						delete(c, "flow")
						delete(g, "flow")
					}
					if !ok || !subset(g, c) {
						return 0, errors.New("profile_client_identity_changed")
					}
					if fmt.Sprint(g["flow"]) != "<nil>" && g["flow"] != "" && g["flow"] != c["flow"] {
						return 0, errors.New("profile_client_flow_changed")
					}
					// Do not silently erase limits introduced outside this profile.
					for _, limit := range []string{"totalGB", "expiryTime", "limitIp", "limitHwid"} {
						if limit == "limitHwid" && globalPolicy[c["id"].(string)] {
							continue
						}
						wantLimit := c[limit]
						if wantLimit == nil {
							wantLimit = float64(0)
						}
						gotLimit := g[limit]
						if gotLimit == nil {
							gotLimit = float64(0)
						}
						if !reflect.DeepEqual(gotLimit, wantLimit) {
							return 0, errors.New("profile_client_limits_changed")
						}
					}
				}
				delete(m, "clients")
				copyExpected := map[string]any{}
				for key, v := range expected {
					if key != "clients" {
						copyExpected[key] = v
					}
				}
				expected = copyExpected
			}
			if !subset(m, expected) {
				return 0, errors.New("profile_transport_mismatch")
			}
		}
		id = int64(rid)
	}
	return id, nil
}

// ProtectedHash includes every sibling, including other managed profiles.
func ProtectedHash(raws []json.RawMessage, owned string) string {
	out := []string{}
	for _, raw := range raws {
		in, e := Object(raw)
		if e != nil {
			return "invalid"
		}
		if in["remark"] == owned {
			continue
		}
		selected := map[string]any{}
		for _, k := range []string{"id", "tag", "protocol", "port", "listen", "enable", "settings", "streamSettings", "sniffing", "expiryTime", "total"} {
			v := in[k]
			if k == "settings" || k == "streamSettings" || k == "sniffing" {
				b, _ := json.Marshal(v)
				if m, e := Object(b); e == nil {
					v = m
				}
			}
			selected[k] = v
		}
		b, _ := json.Marshal(selected)
		out = append(out, string(b))
	}
	sort.Strings(out)
	b, _ := json.Marshal(out)
	return string(b)
}
