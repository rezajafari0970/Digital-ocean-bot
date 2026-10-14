package advancedreality

import (
	"errors"
	"strings"
)

var ErrTransportCombination = errors.New("unsupported_reality_inbound_tcp_mask")

// A native Go JSON decoder matches field names without case sensitivity.
// Reject ambiguous aliases rather than allowing a preflight/native mismatch.
func transportField(m map[string]any, key string) (any, error) {
	var out any
	found := false
	for k, v := range m {
		if !strings.EqualFold(k, key) {
			continue
		}
		if found {
			return nil, ErrJSON
		}
		out = v
		found = true
	}
	return out, nil
}

// rejectRealityInboundMask is a conservative preflight for the reproduced
// server-side half-close incompatibility. It does not validate Xray JSON or
// restrict client outbound fragmentation. Native validation remains required.
func rejectRealityInboundMask(v any) error {
	root, _ := v.(map[string]any)
	raw, e := transportField(root, "inbounds")
	if e != nil {
		return e
	}
	inbounds, _ := raw.([]any)
	for _, raw := range inbounds {
		inbound, _ := raw.(map[string]any)
		raw, e = transportField(inbound, "streamSettings")
		if e != nil {
			return e
		}
		stream, _ := raw.(map[string]any)
		raw, e = transportField(stream, "security")
		if e != nil {
			return e
		}
		security, _ := raw.(string)
		if !strings.EqualFold(security, "reality") {
			continue
		}
		for key, mask := range stream {
			if !strings.EqualFold(key, "finalmask") && !strings.EqualFold(key, "serverFinalMask") {
				continue
			}
			if mask == nil {
				continue
			}
			m, ok := mask.(map[string]any)
			if !ok {
				return ErrTransportCombination
			}
			for k, tcp := range m {
				if !strings.EqualFold(k, "tcp") || tcp == nil {
					continue
				}
				list, ok := tcp.([]any)
				if !ok || len(list) > 0 {
					return ErrTransportCombination
				}
			}
		}
	}
	return nil
}
