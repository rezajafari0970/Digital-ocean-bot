package capacity

// NetOccupancy returns the final desired-capacity occupancy after accounting
// for in-progress replacement-first rotations. Each active replacement pair
// represents one old live resource that will be removed once its replacement
// is ready, so the pair is net-neutral for Desired accounting.
//
// Provider physical capacity is still enforced separately via Snapshot.Available.
func NetOccupancy(localManaged, unmaterialized, providerInUse, activeReplacementPairs int) int {
	if activeReplacementPairs < 0 {
		activeReplacementPairs = 0
	}
	local := localManaged + unmaterialized - activeReplacementPairs
	if local < 0 {
		local = 0
	}
	provider := providerInUse - activeReplacementPairs
	if provider < 0 {
		provider = 0
	}
	if provider > local {
		return provider
	}
	return local
}
