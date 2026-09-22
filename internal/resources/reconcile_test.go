package resources

import "testing"

func TestCompareDetectsDifferences(t *testing.T) {
	local := []Resource{{AccountID: "a", Provider: "digitalocean", ProviderResourceID: "1", Type: "droplet", State: "active"}, {AccountID: "a", Provider: "digitalocean", ProviderResourceID: "2", Type: "droplet", State: "active"}}
	remote := []Resource{{AccountID: "a", Provider: "digitalocean", ProviderResourceID: "1", Type: "droplet", State: "off"}, {AccountID: "a", Provider: "digitalocean", ProviderResourceID: "3", Type: "droplet", State: "active"}}
	diffs := Compare(local, remote)
	seen := map[string]bool{}
	for _, d := range diffs {
		seen[d.Kind] = true
	}
	if !seen[StateChanged] || !seen[UnknownProviderResource] || !seen[MissingProviderResource] {
		t.Fatalf("missing differences: %#v", diffs)
	}
}
