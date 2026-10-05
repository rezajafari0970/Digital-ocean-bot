package providers

import "time"

type ServerState string

const (
	ServerStateUnknown      ServerState = "unknown"
	ServerStateProvisioning ServerState = "provisioning"
	ServerStateReady        ServerState = "ready"
	ServerStateStopping     ServerState = "stopping"
	ServerStateStopped      ServerState = "stopped"
	ServerStateDeleting     ServerState = "deleting"
)

type Account struct {
	ID     string
	Email  string
	Status string
}

type Capacity struct {
	Source        string         `json:",omitempty"`
	PlanAvailable map[string]int `json:",omitempty"`
	ComputeLimit  int
	LimitKnown    bool
	ComputeInUse  int
	ObservedAt    time.Time
}

type Region struct {
	ID        string
	Name      string
	Available bool
}

type Plan struct {
	ID               string
	Name             string
	CPU              int
	MemoryMB         int
	DiskGB           int
	PriceHourly      float64
	PriceMonthly     float64
	PricesByRegion   map[string]PlanPrice `json:",omitempty"`
	Available        bool
	AvailableRegions []string
}

// PlanPrice is region-specific display metadata; it does not authorize spending.
type PlanPrice struct {
	Currency        string
	Hourly          float64
	MonthlyEstimate float64
}

type Image struct {
	ID           string
	Name         string
	Family       string
	Version      string
	Architecture string
	Available    bool
}

type Catalog struct {
	Regions []Region
	Plans   []Plan
	Images  []Image
}

type Server struct {
	ID          string
	Name        string
	State       ServerState
	Ready       bool
	RegionID    string
	PrimaryIPv4 string
	CreatedAt   time.Time
	Tags        []string
	Metadata    map[string]any
}

type CreateServerRequest struct {
	Name              string
	RegionID          string
	PlanID            string
	ImageID           string
	SSHKeyRefs        []string
	SSHAuthorizedKeys []string
	Tags              []string
	Identity          string
}

type MutationOutcome string

const (
	OutcomeAccepted  MutationOutcome = "accepted"
	OutcomeRejected  MutationOutcome = "rejected"
	OutcomeAmbiguous MutationOutcome = "ambiguous"
)

type CreateServerResult struct {
	ServerID    string
	OperationID string
	Outcome     MutationOutcome
}

type SSHKey struct {
	ID          string
	Name        string
	Fingerprint string
}

type Inventory struct {
	Servers    []Server
	Resources  []Resource
	ObservedAt time.Time
}

type Resource struct {
	ID        string
	Type      string
	State     string
	Managed   bool
	CreatedAt time.Time
	Metadata  map[string]any
}

type Observation struct {
	Account    Account
	Capacity   Capacity
	Catalog    Catalog
	Inventory  Inventory
	ObservedAt time.Time
}
