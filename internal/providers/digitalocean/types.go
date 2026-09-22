package digitalocean

import "time"

type Snapshot struct {
	AccountID string
	Provider  string
	Version   int
	Data      map[string]any
	CreatedAt time.Time
}

type DiscoveryResult struct {
	Account   map[string]any
	Limits    map[string]any
	Regions   []map[string]any
	Sizes     []map[string]any
	Images    []map[string]any
	Resources []map[string]any
}
