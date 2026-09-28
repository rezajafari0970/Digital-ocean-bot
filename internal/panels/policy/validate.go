package policy

func Validate(p InboundPolicy) error {
	if p.PanelID == "" || p.ID == "" || p.Protocol == "" || p.DesiredCount < 0 || p.ClientsPerInbound < 0 || p.UserQuotaBytes < 0 || p.UserLifetimeSeconds < 0 || p.DeviceLimit < 0 || p.BulkUserCount < 0 || p.UsersPerSecond < 1 || (p.SNISelectionMode != "scored" && p.SNISelectionMode != "manual") || p.DynamicPortStart < 1 || p.DynamicPortEnd > 65535 || p.DynamicPortStart > p.DynamicPortEnd {
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
