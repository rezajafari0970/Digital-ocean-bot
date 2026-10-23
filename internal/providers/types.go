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
	Condition     string `json:",omitempty"`
	ID            string
	Email         string
	Status        string
	StatusMessage string `json:",omitempty"`
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
	CountryCode string `json:",omitempty"`
	ID          string
	Name        string
	Available   bool
}

type Plan struct {
	DetailsByRegion  map[string]PlanRegionDetails `json:",omitempty"`
	IDsByRegion      map[string]string            `json:",omitempty"`
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

// PlanRegionDetails keeps full provider discovery separate from launch selection.
type PlanRegionDetails struct {
	NativeID       string
	OS             string
	Type           string
	StockAvailable bool
	Supported      bool
	Reason         string
	Quota          *int `json:",omitempty"`
	CPU            int
	MemoryMB       int
	DiskGB         int
	InboundMbps    int
	OutboundMbps   int
}
type CatalogPricing struct {
	Status      string
	Basis       string
	Subsidiary  string
	Currency    string
	Source      string
	CatalogID   int64
	ObservedAt  time.Time
	TaxIncluded bool
}
type PriceComponent struct {
	Name     string
	PlanCode string
	Quantity int
	Hourly   float64
}

// PlanPrice is region-specific display metadata; it does not authorize spending.
type PlanPrice struct {
	Components      []PriceComponent `json:",omitempty"`
	Complete        bool             `json:",omitempty"`
	Scope           string           `json:",omitempty"`
	Currency        string
	Hourly          float64
	MonthlyEstimate float64
}

type Image struct {
	IDsByRegion  map[string]string `json:",omitempty"`
	ID           string
	Name         string
	Family       string
	Version      string
	Architecture string
	Available    bool
}

type Catalog struct {
	CacheVersion string          `json:",omitempty"`
	FetchedAt    time.Time       `json:",omitempty"`
	Pricing      *CatalogPricing `json:",omitempty"`
	Regions      []Region
	Plans        []Plan
	Images       []Image
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
	UpCloudTrialCompatible bool
	Name                   string
	RegionID               string
	PlanID                 string
	ImageID                string
	SSHKeyRefs             []string
	SSHAuthorizedKeys      []string
	Tags                   []string
	Identity               string
}

type MutationOutcome string

const (
	OutcomeAccepted  MutationOutcome = "accepted"
	OutcomeRejected  MutationOutcome = "rejected"
	OutcomeAmbiguous MutationOutcome = "ambiguous"
)

type CreateServerResult struct {
	// ComputeNotDispatched is proof that no compute mutation was sent; auxiliary
	// resources retain their independent journal and recovery obligations.
	ComputeNotDispatched bool
	ServerID             string
	OperationID          string
	Outcome              MutationOutcome
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

// RegionalLaunch is pinned when a deployment is reserved, before any provider mutation.
type RegionalLaunch struct {
	PlanID  string `json:"plan_id"`
	ImageID string `json:"image_id"`
}
