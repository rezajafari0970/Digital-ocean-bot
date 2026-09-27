package policy

func Validate(p InboundPolicy) error {
	if p.PanelID == "" || p.ID == "" || p.Protocol == "" || p.DesiredCount < 0 || p.ClientsPerInbound < 0 || p.DynamicPortStart < 1 || p.DynamicPortEnd > 65535 || p.DynamicPortStart > p.DynamicPortEnd {
		return ErrInvalidPolicy
	}
	seen := map[int]bool{}
	for _, list := range [][]int{p.PreferredPorts, p.ReservedPorts} {
		for _, x := range list {
			if x < 1 || x > 65535 || seen[x] {
				return ErrInvalidPolicy
			}
			seen[x] = true
		}
	}
	return nil
}
