package main

import (
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"strings"
	"time"
)

// Validate the saved two-stage pool without emitting endpoint credentials.
func poolTargets(setting map[string]any, profile ...*residentialperf.Config) (map[string][]string, error) {
	targets := map[string][]string{}
	var c *residentialperf.Config
	if len(profile) > 0 {
		c = profile[0]
	}
	if c != nil && len(c.ExcludedProxyIDs) > 0 {
		b, _ := json.Marshal(setting)
		for _, id := range c.ExcludedProxyIDs {
			if strings.Contains(strings.ToLower(string(b)), strings.ToLower(id)) {
				return nil, fmt.Errorf("excluded proxy remains in native configuration")
			}
		}
	}
	interval, timeout, maxRTT := 10*time.Second, 3*time.Second, 3*time.Second
	laneCount, fastLanes := 5, 4
	if c != nil {
		if e := c.Validate(); e != nil {
			return nil, e
		}
		interval = time.Duration(c.ProbeInterval) * time.Second
		timeout = time.Duration(c.ProbeTimeout) * time.Millisecond
		maxRTT = time.Duration(c.MaxRTT) * time.Millisecond
		laneCount = 10
		fastLanes = c.FastShare / 10
	}
	duration := func(v any, want time.Duration) bool {
		x, ok := v.(string)
		if !ok {
			return false
		}
		d, e := time.ParseDuration(x)
		return e == nil && d == want
	}
	outs := map[string]map[string]any{}
	blocked := ""
	raw, ok := setting["outbounds"].([]any)
	if !ok {
		return nil, fmt.Errorf("outbounds unavailable")
	}
	for _, v := range raw {
		m := v.(map[string]any)
		tag, _ := m["tag"].(string)
		outs[tag] = m
		if strings.HasPrefix(tag, "dob-route-blocked-") {
			blocked = tag
		}
	}
	if blocked == "" || outs[blocked]["protocol"] != "blackhole" {
		return nil, fmt.Errorf("blocking outbound absent")
	}
	routing := setting["routing"].(map[string]any)
	bs, _ := routing["balancers"].([]any)
	pool := map[string]map[string]any{}
	for _, v := range bs {
		m := v.(map[string]any)
		tag, _ := m["tag"].(string)
		if strings.HasPrefix(tag, "dob-route-pool-") {
			pool[tag] = m
		}
	}
	if len(pool) == 0 {
		return targets, nil
	}
	obs, ok := setting["burstObservatory"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("pool observatory missing")
	}
	ping, ok := obs["pingConfig"].(map[string]any)
	if !ok || ping["sampling"] != float64(1) || !duration(ping["interval"], interval) || !duration(ping["timeout"], timeout) || ping["destination"] != "https://connectivitycheck.gstatic.com/generate_204" {
		return nil, fmt.Errorf("observation policy differs")
	}
	observed := map[string]bool{}
	for _, v := range obs["subjectSelector"].([]any) {
		observed[v.(string)] = true
	}
	for _, network := range []string{"tcp", "udp"} {
		outer, ok := pool["dob-route-pool-"+network]
		if !ok {
			targets["@pool:"+network] = []string{blocked}
			continue
		}
		strategy, _ := outer["strategy"].(map[string]any)
		lanes, _ := outer["selector"].([]any)
		if len(lanes) != laneCount || strategy["type"] != "random" || outer["fallbackTag"] != blocked {
			return nil, fmt.Errorf("weighted lane configuration differs")
		}
		fast, all := 0, 0
		for _, v := range lanes {
			tag := v.(string)
			o := outs[tag]
			if o["protocol"] != "loopback" {
				return nil, fmt.Errorf("non-loopback pool lane")
			}
			st := o["settings"].(map[string]any)
			switch st["inboundTag"] {
			case "dob-route-pool-in-" + network + "-fast":
				fast++
			case "dob-route-pool-in-" + network + "-all":
				all++
			default:
				return nil, fmt.Errorf("lane target differs")
			}
			targets["@pool:"+network] = append(targets["@pool:"+network], tag)
		}
		if fast != fastLanes || all != laneCount-fastLanes {
			return nil, fmt.Errorf("profile weights differ")
		}
		for _, kind := range []string{"fast", "all"} {
			b := pool["dob-route-pool-"+network+"-"+kind]
			strategy, _ := b["strategy"].(map[string]any)
			settings, _ := strategy["settings"].(map[string]any)
			selectors, _ := b["selector"].([]any)
			expected := len(selectors)
			if kind == "fast" {
				expected = (expected + 2) / 3
				if c != nil && c.FastCount > 0 {
					expected = min(c.FastCount, len(selectors))
				}
			}
			if len(selectors) == 0 || strategy["type"] != "leastLoad" || settings["expected"] != float64(expected) || !duration(settings["maxRTT"], maxRTT) || b["fallbackTag"] != blocked {
				return nil, fmt.Errorf("healthy selection differs")
			}
			key := "@inner:" + network + ":" + kind
			targets[key] = []string{blocked}
			for _, v := range selectors {
				tag := v.(string)
				o := outs[tag]
				if !observed[tag] || !strings.HasPrefix(tag, "residential-ads-") || (o["protocol"] != "socks" && o["protocol"] != "http") || (network == "udp" && o["protocol"] != "socks") {
					return nil, fmt.Errorf("unobserved or unsupported proxy selected")
				}
				targets[key] = append(targets[key], tag)
			}
		}
	}
	return targets, nil
}
