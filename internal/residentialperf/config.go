// Package residentialperf owns reversible, bounded server-side tuning.
package residentialperf

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

type Config struct {
	FastShare         int                `json:"fast_share"`
	FastCount         int                `json:"fast_count"`
	ProbeInterval     int                `json:"probe_interval_seconds"`
	ProbeTimeout      int                `json:"probe_timeout_ms"`
	MaxRTT            int                `json:"max_rtt_ms"`
	DNSMode           string             `json:"dns_mode"`
	GatewayIPv4       bool               `json:"gateway_ipv4"`
	TCPOptions        bool               `json:"tcp_options"`
	FastOpen          bool               `json:"tcp_fast_open"`
	KeepAliveIdle     int                `json:"keepalive_idle_seconds"`
	KeepAliveInterval int                `json:"keepalive_interval_seconds"`
	TCPUserTimeout    int                `json:"tcp_user_timeout_ms"`
	RoutingStrategy   string             `json:"routing_strategy"`
	BufferKB          int                `json:"buffer_kb"`
	Costs             map[string]float64 `json:"costs,omitempty"`
}

func Balanced() Config {
	return Config{FastShare: 80, FastCount: 3, ProbeInterval: 10, ProbeTimeout: 3000, MaxRTT: 3000, DNSMode: "tcp", GatewayIPv4: true, TCPOptions: true, KeepAliveIdle: 30, KeepAliveInterval: 20, TCPUserTimeout: 20000, RoutingStrategy: "preserve", BufferKB: 64}
}

var UUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (c Config) Validate() error {
	if c.FastShare < 50 || c.FastShare > 90 || c.FastShare%10 != 0 || c.FastCount < 0 || c.FastCount > 16 {
		return errors.New("fast share must be 50–90 in steps of 10; fast count must be 0–16")
	}
	if c.ProbeInterval < 10 || c.ProbeInterval > 60 || c.ProbeTimeout < 1000 || c.ProbeTimeout > 5000 || c.MaxRTT < 500 || c.MaxRTT > c.ProbeTimeout {
		return errors.New("invalid bounded probe interval, timeout or maximum RTT")
	}
	if c.DNSMode != "tcp" && c.DNSMode != "doh" {
		return errors.New("DNS mode must be tcp or doh")
	}
	if c.RoutingStrategy != "preserve" && c.RoutingStrategy != "AsIs" {
		return errors.New("routing strategy must be preserve or AsIs")
	}
	if c.BufferKB != 0 && c.BufferKB != 32 && c.BufferKB != 64 && c.BufferKB != 128 && c.BufferKB != 256 && c.BufferKB != 512 {
		return errors.New("buffer must be inherited (0) or 32/64/128/256/512 KiB")
	}
	if c.KeepAliveIdle < 15 || c.KeepAliveIdle > 120 || c.KeepAliveInterval < 10 || c.KeepAliveInterval > 60 || c.TCPUserTimeout < 10000 || c.TCPUserTimeout > 60000 {
		return errors.New("TCP options are outside tested bounds")
	}
	if len(c.Costs) > 100 {
		return errors.New("too many proxy costs")
	}
	for id, v := range c.Costs {
		if !UUID.MatchString(id) || math.IsNaN(v) || math.IsInf(v, 0) || v < 0.25 || v > 4 {
			return errors.New("proxy costs require UUIDs and values from 0.25 to 4")
		}
	}
	return nil
}

type Value struct {
	Present bool `json:"present"`
	Data    any  `json:"value,omitempty"`
}
type Fields struct {
	Strategy      Value `json:"routing_strategy"`
	Buffer        Value `json:"buffer"`
	PolicyPresent bool  `json:"policy_present"`
	LevelsPresent bool  `json:"levels_present"`
	LevelPresent  bool  `json:"level_present"`
}

func value(m map[string]any, k string) Value        { v, ok := m[k]; return Value{ok, v} }
func obj(m map[string]any, k string) map[string]any { v, _ := m[k].(map[string]any); return v }
func Capture(m map[string]any) Fields {
	p := obj(m, "policy")
	l := obj(p, "levels")
	z := obj(l, "0")
	return Fields{value(obj(m, "routing"), "domainStrategy"), value(z, "bufferSize"), p != nil, l != nil, z != nil}
}
func equal(a, b Value) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// A lost response may leave last-observed or durably-planned values.
func CheckOwned(c Fields, alternatives ...*Fields) error {
	match := func(v Value, get func(*Fields) Value) bool {
		for _, f := range alternatives {
			if f != nil && equal(v, get(f)) {
				return true
			}
		}
		return false
	}
	if !match(c.Strategy, func(f *Fields) Value { return f.Strategy }) || !match(c.Buffer, func(f *Fields) Value { return f.Buffer }) {
		return errors.New("performance field conflict: an owned field changed outside this experiment")
	}
	return nil
}
func ensure(m map[string]any, k string) map[string]any {
	if v := obj(m, k); v != nil {
		return v
	}
	v := map[string]any{}
	m[k] = v
	return v
}
func set(m map[string]any, k string, v Value) {
	if v.Present {
		m[k] = v.Data
	} else {
		delete(m, k)
	}
}
func Restore(m map[string]any, f Fields) {
	set(ensure(m, "routing"), "domainStrategy", f.Strategy)
	p := obj(m, "policy")
	l := obj(p, "levels")
	z := obj(l, "0")
	if f.Buffer.Present {
		ensure(ensure(ensure(m, "policy"), "levels"), "0")["bufferSize"] = f.Buffer.Data
		return
	}
	delete(z, "bufferSize")
	if len(z) == 0 && !f.LevelPresent {
		delete(l, "0")
	}
	if len(l) == 0 && !f.LevelsPresent {
		delete(p, "levels")
	}
	if len(p) == 0 && !f.PolicyPresent {
		delete(m, "policy")
	}
}
func Apply(m map[string]any, c *Config) error {
	if c == nil {
		return nil
	}
	if e := c.Validate(); e != nil {
		return e
	}
	if c.RoutingStrategy == "AsIs" {
		ensure(m, "routing")["domainStrategy"] = "AsIs"
	}
	if c.BufferKB > 0 {
		ensure(ensure(ensure(m, "policy"), "levels"), "0")["bufferSize"] = c.BufferKB
	}
	if c.DNSMode == "doh" {
		ensure(m, "dns")["servers"] = []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"}
	}
	raw, _ := m["outbounds"].([]any)
	for _, v := range raw {
		o, ok := v.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := o["tag"].(string)
		res := strings.HasPrefix(tag, "residential-ads-")
		if !res && tag != "dob-route-direct" {
			continue
		}
		if c.GatewayIPv4 || c.TCPOptions {
			s := ensure(ensure(o, "streamSettings"), "sockopt")
			if c.GatewayIPv4 {
				s["domainStrategy"] = "ForceIPv4"
			}
			if c.TCPOptions {
				s["tcpFastOpen"] = c.FastOpen
				s["tcpKeepAliveIdle"] = c.KeepAliveIdle
				s["tcpKeepAliveInterval"] = c.KeepAliveInterval
				s["tcpUserTimeout"] = c.TCPUserTimeout
			}
		}
	}
	return nil
}
func DurationMS(ms int) string { return fmt.Sprintf("%dms", ms) }
