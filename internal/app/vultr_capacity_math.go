package app

func vultrProbeEvidence(oldLimit, apiCount int) (newLimit, inUse int) {
	newLimit = oldLimit + 1
	if apiCount > newLimit {
		newLimit = apiCount
	}
	inUse = apiCount
	if inUse < oldLimit+1 {
		inUse = oldLimit + 1
	}
	return newLimit, inUse
}
