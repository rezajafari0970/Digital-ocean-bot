package kamatera

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"strings"
	"time"
)

type Factory struct{}

func (Factory) Name() string { return "kamatera" }
func (Factory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "kamatera", DisplayName: "Kamatera", CredentialLabel: "Kamatera API credentials", CredentialFields: []providers.CredentialField{{Key: "client_id", Label: "API Client ID", Secret: true}, {Key: "secret", Label: "API Secret", Secret: true}}, CredentialHelp: "Create an API key in Kamatera Console. Region, predefined and capability-verified suggested plans, and supported Ubuntu images are loaded through this key's access. Validation creates no server. Live public base estimates are matched to each configuration and region. Servers use hourly billing; traffic is charged separately. Billing totals, account-specific quotes and exact quota are not exposed by this integration.", Status: "ready", Defaults: cloudutil.Defaults()}
}
func (Factory) Open(ctx context.Context, r providers.OpenRequest) (providers.Driver, error) {
	if r.AccountID == "" || r.HTTPClient == nil || r.Credentials == nil {
		return nil, errors.New("Kamatera: invalid open request")
	}
	c, e := credentials(ctx, r.Credentials)
	if e != nil {
		return nil, e
	}
	if r.ExpectedExternalID != "" && r.ExpectedExternalID != fingerprint(c.ClientID) {
		return nil, problem(providers.ErrorAuthentication, "account", "CREDENTIAL_SCOPE_CHANGED", 0)
	}
	return &Driver{client: &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: r.Credentials, expected: r.ExpectedExternalID}, accountID: r.AccountID, resources: r.Resources, absence: r.Absence}, nil
}

type Driver struct {
	client    *client
	accountID string
	resources providers.AccountResources
	absence   providers.AbsenceVerifier
}

func (d *Driver) Name() string { return "kamatera" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, Inventory: true, InlineSSHKeys: true}
}
func (d *Driver) Health(ctx context.Context) error { _, e := d.Account(ctx); return e }

type serverData struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Region   string `json:"datacenter"`
	Power    string `json:"power"`
	Networks []struct {
		Network string   `json:"network"`
		IPs     []string `json:"ips"`
	} `json:"networks"`
}

func validateServers(xs []serverData) error {
	if xs == nil {
		return problem(providers.ErrorUnavailable, "inventory", "INVALID_INVENTORY", 200)
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if !uuidPattern.MatchString(x.ID) || seen[x.ID] || !regionPattern.MatchString(x.Region) || x.Name == "" || x.Power == "" {
			return problem(providers.ErrorUnavailable, "inventory", "INVALID_SERVER_IDENTITY", 200)
		}
		seen[x.ID] = true
	}
	return nil
}
func (d *Driver) basicServers(ctx context.Context) ([]serverData, error) {
	var xs []serverData
	if e := d.client.do(ctx, "GET", "/service/servers", nil, &xs); e != nil {
		return nil, e
	}
	return xs, validateServers(xs)
}
func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	if _, e := d.basicServers(ctx); e != nil {
		return providers.Account{}, e
	}
	c, e := credentials(ctx, d.client.credentials)
	if e != nil {
		return providers.Account{}, e
	}
	// API-key scope is explicit: the published API does not expose a customer UUID.
	return providers.Account{ID: fingerprint(c.ClientID), Status: "active", StatusMessage: "Verified API-key scope; account-wide quota unavailable"}, nil
}
func capacity(n int) providers.Capacity {
	return providers.Capacity{ComputeInUse: n, LimitKnown: false, Source: "kamatera_api_key_scope_usage_only", ObservedAt: time.Now().UTC()}
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	xs, e := d.basicServers(ctx)
	return capacity(len(xs)), e
}
func (d *Driver) normalize(x serverData) providers.Server {
	ip := ""
	for _, n := range x.Networks {
		if n.Network == "wan" || strings.HasPrefix(n.Network, "wan-") {
			for _, v := range n.IPs {
				if ip == "" {
					ip = cloudutil.PublicIPv4(v)
				}
			}
		}
	}
	state := providers.ServerStateUnknown
	switch x.Power {
	case "on":
		state = providers.ServerStateProvisioning
		if ip != "" {
			state = providers.ServerStateReady
		}
	case "off":
		state = providers.ServerStateStopped
	}
	return providers.Server{ID: x.ID, Name: x.Name, RegionID: x.Region, State: state, Ready: state == providers.ServerStateReady, PrimaryIPv4: ip, Metadata: map[string]any{"provider_state": x.Power}}
}
func (d *Driver) rawServers(ctx context.Context, selector map[string]string) ([]serverData, error) {
	var xs []serverData
	if e := d.client.do(ctx, "POST", "/service/server/info", selector, &xs); e != nil {
		var pe *providers.Error
		if len(selector) == 1 && selector["name"] == ".*" && errors.As(e, &pe) && pe.Code == "EMPTY_WILDCARD_SELECTION" {
			basic, verifyErr := d.basicServers(ctx)
			if verifyErr != nil {
				return nil, verifyErr
			}
			if len(basic) == 0 {
				return []serverData{}, nil
			}
			return nil, problem(providers.ErrorUnavailable, "inventory", "INVENTORY_DISAGREEMENT", 200)
		}
		return nil, e
	}
	return xs, validateServers(xs)
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	if !uuidPattern.MatchString(id) {
		return providers.Server{}, problem(providers.ErrorInvalidRequest, "get_server", "INVALID_SERVER_ID", 0)
	}
	xs, e := d.rawServers(ctx, map[string]string{"id": id})
	if e != nil {
		return providers.Server{}, e
	}
	if len(xs) == 0 {
		basic, e := d.basicServers(ctx)
		if e != nil {
			return providers.Server{}, e
		}
		for _, x := range basic {
			if x.ID == id {
				return providers.Server{}, problem(providers.ErrorUnavailable, "get_server", "INVENTORY_DISAGREEMENT", 200)
			}
		}
		if e = providers.ConfirmAbsence(ctx, d.absence, id); e != nil {
			return providers.Server{}, e
		}
		return providers.Server{}, problem(providers.ErrorNotFound, "get_server", "SERVER_ABSENT", 404)
	}
	if len(xs) != 1 || xs[0].ID != id {
		return providers.Server{}, problem(providers.ErrorUnavailable, "get_server", "SERVER_SCOPE_MISMATCH", 200)
	}
	if d.absence != nil {
		if e = d.absence.Present(ctx, id); e != nil {
			return providers.Server{}, e
		}
	}
	return d.normalize(xs[0]), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	xs, e := d.rawServers(ctx, map[string]string{"name": ".*"})
	if e != nil {
		return nil, e
	}
	out := make([]providers.Server, 0, len(xs))
	for _, x := range xs {
		n := d.normalize(x)
		out = append(out, n)
	}
	if d.resources != nil {
		rs, e := d.resources.List(ctx)
		if e != nil {
			return nil, e
		}
		for i := range out {
			for _, r := range rs {
				if r.Attempted && !r.Complete && r.ID == out[i].ID && r.Name == out[i].Name && strings.HasPrefix(r.Scope, "server:") {
					out[i].Tags = []string{"managed-by-digital-ocean-bot", "dob-account-" + d.accountID, strings.TrimPrefix(r.Scope, "server:")}
				}
			}
		}
	}
	return out, nil
}
func (d *Driver) Inventory(ctx context.Context) (providers.Inventory, error) {
	xs, e := d.ListServers(ctx)
	return providers.Inventory{Servers: xs, Resources: []providers.Resource{}, ObservedAt: time.Now().UTC()}, e
}
func (d *Driver) observe(ctx context.Context, full bool) (providers.Observation, error) {
	a, e := d.Account(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	inv, e := d.Inventory(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	cat := providers.Catalog{}
	if full {
		cat, e = d.Catalog(ctx)
		if e != nil {
			return providers.Observation{}, e
		}
	}
	return providers.Observation{Account: a, Capacity: capacity(len(inv.Servers)), Catalog: cat, Inventory: inv, ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Observe(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, true)
}
func (d *Driver) ObserveFast(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, false)
}

var _ providers.Factory = Factory{}
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.FastObservationReader = (*Driver)(nil)
var _ providers.InventoryReader = (*Driver)(nil)
var _ providers.VersionedCatalogReader = (*Driver)(nil)
