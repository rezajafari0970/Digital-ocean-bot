package vultr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type Factory struct{}

func (Factory) Name() string { return "vultr" }
func (Factory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "vultr", DisplayName: "Vultr", CredentialLabel: "API key", Status: "development", Defaults: providers.Defaults{Images: providers.ImagePolicy{Family: "ubuntu", Versions: []string{"26.04", "24.04", "22.04"}}, LifetimeMinMinutes: 90, LifetimeMaxMinutes: 120, DesiredServers: 5, BuildSpacingMinMinutes: 1, BuildSpacingMaxMinutes: 3, MaxConcurrent: 1, FallbackAnyRegion: true}}
}
func (Factory) Open(ctx context.Context, req providers.OpenRequest) (providers.Driver, error) {
	if strings.TrimSpace(req.AccountID) == "" || req.HTTPClient == nil || req.Credentials == nil {
		return nil, fmt.Errorf("vultr driver: invalid open request")
	}
	b, err := req.Credentials.Get(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, fmt.Errorf("vultr driver: empty credential")
	}
	return &Driver{client: NewClient(req.HTTPClient, string(b))}, nil
}

type Driver struct{ client *Client }

func (d *Driver) Name() string { return "vultr" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, SSHKeys: true, Inventory: true}
}
func (d *Driver) Health(ctx context.Context) error {
	var x accountResponse
	return normalizeError("health", d.client.do(ctx, http.MethodGet, "/account", nil, &x))
}

func normalizeError(op string, err error) error {
	if err == nil {
		return nil
	}
	class := providers.ErrorTransport
	var h HTTPError
	if errors.As(err, &h) {
		switch h.Status {
		case 401:
			class = providers.ErrorAuthentication
		case 403:
			class = providers.ErrorPermissionDenied
		case 404:
			class = providers.ErrorNotFound
		case 409:
			class = providers.ErrorInvalidRequest
		case 422:
			class = providers.ErrorInvalidRequest
		case 429:
			class = providers.ErrorRateLimited
		default:
			if h.Status >= 500 {
				class = providers.ErrorTransport
			}
		}
	}
	pe := &providers.Error{Class: class, Operation: op, Message: err.Error(), Cause: err}
	if errors.As(err, &h) {
		pe.StatusCode = h.Status
		pe.RetryAfter = h.RetryAfter
	}
	return pe
}

var _ providers.Driver = (*Driver)(nil)
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.CatalogReader = (*Driver)(nil)
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.SSHKeyDriver = (*Driver)(nil)
var _ providers.ObservationReader = (*Driver)(nil)
