package providers

import (
	"context"
	"net/http"
)

type Capabilities struct {
	Account   bool
	Catalog   bool
	Compute   bool
	SSHKeys   bool
	Inventory bool
}

type Driver interface {
	Name() string
	Capabilities() Capabilities
	Health(context.Context) error
}

type AccountReader interface {
	Account(context.Context) (Account, error)
	Capacity(context.Context) (Capacity, error)
}

type CatalogReader interface {
	Catalog(context.Context) (Catalog, error)
}

type ComputeDriver interface {
	CreateServer(context.Context, CreateServerRequest) (CreateServerResult, error)
	GetServer(context.Context, string) (Server, error)
	ListServers(context.Context) ([]Server, error)
	DeleteServer(context.Context, string) error
	FindServerByIdentity(context.Context, string) ([]Server, error)
}

type SSHKeyDriver interface {
	CreateSSHKey(context.Context, string, string) (SSHKey, error)
	DeleteSSHKey(context.Context, string) error
}

type InventoryReader interface {
	Inventory(context.Context) (Inventory, error)
}

type ObservationReader interface {
	Observe(context.Context) (Observation, error)
}

type CredentialSource interface {
	Get(context.Context) ([]byte, error)
}

type OpenRequest struct {
	AccountID   string
	HTTPClient  *http.Client
	Credentials CredentialSource
}

type Factory interface {
	Name() string
	Metadata() Metadata
	Open(context.Context, OpenRequest) (Driver, error)
}
