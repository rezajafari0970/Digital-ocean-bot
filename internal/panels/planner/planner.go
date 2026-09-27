package planner

import (
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"sort"
)

func Plan(desired []DesiredInbound, actual []inventory.InboundRecord, policy Policy) ([]Action, error) {
	byKey := make(map[string]DesiredInbound, len(desired))
	ports := map[int]string{}
	for _, d := range desired {
		if d.Key == "" || d.Remark == "" || d.Key != d.Remark || d.Port < 1 || d.Port > 65535 || d.Protocol == "" {
			return nil, fmt.Errorf("invalid desired inbound %q", d.Key)
		}
		if _, ok := byKey[d.Key]; ok {
			return nil, fmt.Errorf("duplicate desired key %q", d.Key)
		}
		if prev, ok := ports[d.Port]; ok {
			return nil, fmt.Errorf("port collision %d between %s and %s", d.Port, prev, d.Key)
		}
		byKey[d.Key] = d
		ports[d.Port] = d.Key
	}
	actualByRemark := map[string]inventory.InboundRecord{}
	for _, a := range actual {
		actualByRemark[a.Remark] = a
	}
	out := make([]Action, 0, len(desired)+len(actual))
	for _, d := range desired {
		a, ok := actualByRemark[d.Remark]
		if !ok {
			out = append(out, Action{Kind: ActionCreate, Key: d.Key, Desired: d, Reason: "missing"})
			continue
		}
		delete(actualByRemark, d.Remark)
		if differs(d, a) {
			out = append(out, Action{Kind: ActionUpdate, Key: d.Key, RemoteID: a.RemoteID, Desired: d, Reason: "drift"})
		} else {
			out = append(out, Action{Kind: ActionNoop, Key: d.Key, RemoteID: a.RemoteID, Desired: d, Reason: "converged"})
		}
	}
	for _, a := range actualByRemark {
		kind := ActionNoop
		reason := "unmanaged"
		if policy.AllowDelete {
			kind = ActionDelete
			reason = "not desired"
		}
		out = append(out, Action{Kind: kind, RemoteID: a.RemoteID, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Key < out[j].Key
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

func differs(d DesiredInbound, a inventory.InboundRecord) bool {
	return d.Protocol != a.Protocol || d.Port != a.Port || d.Listen != a.Listen || d.Enabled != a.Enabled || d.Transport != a.Transport || d.Security != a.Security
}
