package resources

const (
	Known                   = "KNOWN"
	UnknownProviderResource = "UNKNOWN_RESOURCE"
	MissingProviderResource = "MISSING_RESOURCE"
	StateChanged            = "STATE_CHANGED"
)

func Compare(local, provider []Resource) []Difference {
	localByKey := make(map[string]Resource, len(local))
	remoteByKey := make(map[string]Resource, len(provider))
	for _, r := range local {
		localByKey[key(r)] = r
	}
	for _, r := range provider {
		remoteByKey[key(r)] = r
	}
	out := make([]Difference, 0)
	for k, remote := range remoteByKey {
		localResource, ok := localByKey[k]
		if !ok {
			out = append(out, Difference{Kind: UnknownProviderResource, Resource: remote})
			continue
		}
		if localResource.State != remote.State {
			out = append(out, Difference{Kind: StateChanged, Resource: remote})
		}
	}
	for k, localResource := range localByKey {
		if _, ok := remoteByKey[k]; !ok {
			out = append(out, Difference{Kind: MissingProviderResource, Resource: localResource})
		}
	}
	return out
}

func key(r Resource) string {
	return r.Provider + ":" + r.AccountID + ":" + r.Type + ":" + r.ProviderResourceID
}
