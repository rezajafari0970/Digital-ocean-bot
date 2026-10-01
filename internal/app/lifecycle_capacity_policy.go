package app

func shouldDeleteFirstAtDesired(desired, managed, retiring int, isOldestExpiring bool) bool {
	return desired > 0 && managed >= desired && retiring == 0 && isOldestExpiring
}
