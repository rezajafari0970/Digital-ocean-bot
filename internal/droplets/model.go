package droplets

import "time"

type State string

const (
	Requested    State = "REQUESTED"
	Planned      State = "PLANNED"
	Creating     State = "CREATING"
	Active       State = "ACTIVE"
	Provisioning State = "PROVISIONING"
	Ready        State = "READY"
	Expiring     State = "EXPIRING"
	Retiring     State = "RETIRING"
	Deleting     State = "DELETING"
	Deleted      State = "DELETED"
)

type Profile struct {
	Name        string
	Region      string
	Regions     []string
	Size        string
	Image       string
	Lifetime    time.Duration
	Provision   string
	IdentityTag string
}

type Droplet struct {
	ID         string
	AccountID  string
	ProviderID string
	Profile    Profile
	State      State
}
