package providers

import "time"

type ResourceType string

const (
	ResourceDroplet  ResourceType = "droplet"
	ResourceVolume   ResourceType = "volume"
	ResourceFirewall ResourceType = "firewall"
)

type AccountInfo struct {
	ID     string
	Email  string
	Status string
}

type Limits struct {
	DropletLimit int
}

type Resource struct {
	ID         string
	ProviderID string
	AccountID  string
	Type       ResourceType
	State      string
	Managed    bool
	CreatedAt  time.Time
}

type CreateRequest struct {
	Type ResourceType
	Name string
}

type Operation struct {
	ID               string
	ProviderActionID string
	AccountID        string
	State            string
}
