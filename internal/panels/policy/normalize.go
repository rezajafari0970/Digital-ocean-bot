package policy

func Normalize(p InboundPolicy) InboundPolicy {
	if p.UsersPerSecond == 0 {
		p.UsersPerSecond = 1
	}
	if p.SNISelectionMode == "" {
		p.SNISelectionMode = "scored"
	}
	return p
}
