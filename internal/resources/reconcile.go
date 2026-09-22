package resources

const (
	Known                   = "KNOWN"
	UnknownProviderResource = "UNKNOWN_RESOURCE"
	MissingProviderResource = "MISSING_RESOURCE"
	StateChanged            = "STATE_CHANGED"
)

func Compare(local []Resource, provider []Resource) []Difference {
	return []Difference{}
}
