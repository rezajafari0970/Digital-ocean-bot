package policy

import (
	"errors"
	"fmt"
)

var ErrInsufficientPorts = errors.New("insufficient free ports")

func AllocatePorts(p InboundPolicy, occupied []int) ([]int, error) {
	if p.DesiredCount < 0 || p.DynamicPortStart < 1 || p.DynamicPortEnd > 65535 || p.DynamicPortStart > p.DynamicPortEnd {
		return nil, fmt.Errorf("invalid port policy")
	}
	used := map[int]bool{}
	for _, x := range occupied {
		if x >= 1 && x <= 65535 {
			used[x] = true
		}
	}
	for _, x := range p.ReservedPorts {
		if x >= 1 && x <= 65535 {
			used[x] = true
		}
	}
	out := make([]int, 0, p.DesiredCount)
	add := func(x int) {
		if len(out) < p.DesiredCount && x >= 1 && x <= 65535 && !used[x] {
			used[x] = true
			out = append(out, x)
		}
	}
	for _, x := range p.PreferredPorts {
		add(x)
	}
	for x := p.DynamicPortStart; x <= p.DynamicPortEnd && len(out) < p.DesiredCount; x++ {
		add(x)
	}
	if len(out) != p.DesiredCount {
		return nil, fmt.Errorf("%w: need=%d got=%d", ErrInsufficientPorts, p.DesiredCount, len(out))
	}
	return out, nil
}
