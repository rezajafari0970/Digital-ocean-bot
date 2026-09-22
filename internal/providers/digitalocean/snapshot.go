package digitalocean

import "time"

type Snapshot struct {
	AccountID string
	Provider  string
	Version   int64
	Data      DiscoveryResult
	CreatedAt time.Time
}

func NewSnapshot(accountID string, version int64, result DiscoveryResult) Snapshot {
	return Snapshot{AccountID: accountID, Provider: "digitalocean", Version: version, Data: result, CreatedAt: time.Now().UTC()}
}
