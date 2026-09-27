package panels

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnsupported  = errors.New("panel capability unsupported")
	ErrUnavailable  = errors.New("panel unavailable")
	ErrUnauthorized = errors.New("panel unauthorized")
)

type Instance struct {
	ID            string
	AccountID     string
	DropletID     string
	Driver        string
	BaseURL       string
	AuthSecretRef string
	Version       string
	Enabled       bool
}

type Capabilities struct {
	InventorySlim bool `json:"inventory_slim"`
	InventoryFull bool `json:"inventory_full"`

	InboundRead  bool `json:"inbound_read"`
	InboundWrite bool `json:"inbound_write"`

	ClientRead  bool `json:"client_read"`
	ClientWrite bool `json:"client_write"`

	TrafficRead bool `json:"traffic_read"`

	ServerStatus bool `json:"server_status"`
	RealityScan  bool `json:"reality_scan"`

	BearerAuth bool `json:"bearer_auth"`
}

type Discovery struct {
	Driver       string
	Version      string
	Capabilities Capabilities
	ObservedAt   time.Time

	// HTTP status / runtime evidence only.
	// Never credentials or response bodies containing secrets.
	Evidence map[string]int
}

type Driver interface {
	Name() string

	Discover(
		context.Context,
		Instance,
	) (Discovery, error)

	Health(
		context.Context,
		Instance,
	) error
}
