package main

import (
	"fmt"
	"strings"
)

// Validate the saved two-stage pool without emitting endpoint credentials.
func poolTargets(setting map[string]any) (map[string][]string, error) {
	targets := map[string][]string{}
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
	if blocked == "" {
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
	if !ok || ping["sampling"] != float64(1) || ping["interval"] != "10s" || ping["timeout"] != "3s" || ping["destination"] != "https://connectivitycheck.gstatic.com/generate_204" {
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
		if len(lanes) != 5 || strategy["type"] != "random" || outer["fallbackTag"] != blocked {
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
		if fast != 4 || all != 1 {
			return nil, fmt.Errorf("80/20 weights differ")
		}
		for _, kind := range []string{"fast", "all"} {
			b := pool["dob-route-pool-"+network+"-"+kind]
			strategy, _ := b["strategy"].(map[string]any)
			settings, _ := strategy["settings"].(map[string]any)
			selectors, _ := b["selector"].([]any)
			expected := len(selectors)
			if kind == "fast" {
				expected = (expected + 2) / 3
			}
			if len(selectors) == 0 || strategy["type"] != "leastLoad" || settings["expected"] != float64(expected) || settings["maxRTT"] != "3s" || b["fallbackTag"] != blocked {
				return nil, fmt.Errorf("healthy selection differs")
			}
			key := "@inner:" + network + ":" + kind
			targets[key] = []string{blocked}
			for _, v := range selectors {
				tag := v.(string)
				o := outs[tag]
				if !observed[tag] || !strings.HasPrefix(tag, "residential-ads-") || (network == "udp" && o["protocol"] != "socks") {
					return nil, fmt.Errorf("unobserved or unsupported proxy selected")
				}
				targets[key] = append(targets[key], tag)
			}
		}
	}
	return targets, nil
}
