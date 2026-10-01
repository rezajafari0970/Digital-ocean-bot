package app

func desiredAllowsCreate(desired, managed, preCreate int) bool {
	return desired > 0 && managed >= 0 && preCreate >= 0 && managed+preCreate < desired
}

func desiredAllowsReservedCreate(desired, managed, preCreate int) bool {
	return desired > 0 && managed >= 0 && preCreate > 0 && managed+preCreate <= desired
}

func desiredEffectiveManaged(localManaged, providerInUse int) int {
	if providerInUse > localManaged {
		return providerInUse
	}
	return localManaged
}
