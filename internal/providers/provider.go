package providers

import "context"

type Provider interface {
	Health(ctx context.Context) error
	GetAccount(ctx context.Context) (*AccountInfo, error)
	GetLimits(ctx context.Context) (*Limits, error)
	ListResources(ctx context.Context) ([]Resource, error)
	GetResource(ctx context.Context, id string) (*Resource, error)
	CreateResource(ctx context.Context, req CreateRequest) (*Operation, error)
	DeleteResource(ctx context.Context, id string) (*Operation, error)
	GetOperation(ctx context.Context, id string) (*Operation, error)
}
