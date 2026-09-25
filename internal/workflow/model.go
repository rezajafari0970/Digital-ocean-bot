package workflow

import "time"

type State string

const (
	Planned            State = "PLANNED"
	Creating           State = "CREATING"
	WaitingResource    State = "WAITING_RESOURCE"
	Provisioning       State = "PROVISIONING"
	ImportingDatabase  State = "IMPORTING_DATABASE"
	ConfiguringPanel   State = "CONFIGURING_PANEL"
	RegisteringClients State = "REGISTERING_CLIENTS"
	RegisteringTraffic State = "REGISTERING_TRAFFIC"
	Ready              State = "READY"
	Failed             State = "FAILED"
)

type Deployment struct {
	ID          string
	AccountID   string
	ProfileID   string
	DropletID   string
	ProviderID  string
	State       State
	CurrentStep string
	Attempt     int
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
type Request struct {
	DeploymentID string
	AccountID    string
	ProfileID    string
	ClientCount  int
	InboundID    int
	EmailPrefix  string
}
