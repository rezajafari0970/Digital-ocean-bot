package resources

import "time"

type Resource struct {
	ID                 string
	AccountID          string
	Provider           string
	ProviderResourceID string
	Type               string
	State              string
	Managed            bool
	Metadata           map[string]any
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Difference struct {
	Kind     string
	Resource Resource
}
