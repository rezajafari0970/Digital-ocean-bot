package scheduler

func Plan(cap Capacity, policy Policy) Decision {
	if policy.MaxServers > 0 && cap.Active+cap.Creating+cap.Provisioning >= policy.MaxServers {
		return BlockCapacity
	}
	if cap.Available() <= 0 {
		return BlockCapacity
	}
	return AllowCreate
}
