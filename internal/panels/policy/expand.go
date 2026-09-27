package policy

import (
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/planner"
)

func Expand(p InboundPolicy, occupied []int) ([]planner.DesiredInbound, error) {
	if err := Validate(p); err != nil {
		return nil, err
	}
	ports, err := AllocatePorts(p, occupied)
	if err != nil {
		return nil, err
	}
	out := make([]planner.DesiredInbound, 0, len(ports))
	for i, port := range ports {
		key := fmt.Sprintf("dob:%s:%06d", p.ID, i+1)
		out = append(out, planner.DesiredInbound{Key: key, Remark: key, Protocol: p.Protocol, Port: port, Listen: p.Listen, Enabled: p.Enabled, Transport: p.Transport, Security: p.Security, Managed: true})
	}
	return out, nil
}
