package app

func shouldDeleteFirstAtDesired(desired, managed, retiring int, isOldestExpiring bool) bool {
	return desired > 0 && managed >= desired && retiring == 0 && isOldestExpiring
}

// When the account is already below its hard Desired ceiling, the free slot
// belongs to scheduler/backfill. An unrelated EXPIRING resource must not claim
// that deficit as its replacement; doing so causes another delete after the
// replacement becomes ready and can transiently oversupply the account.
func shouldWaitForDeficitBackfill(desired, managed int) bool {
	return desired > 0 && managed < desired
}
