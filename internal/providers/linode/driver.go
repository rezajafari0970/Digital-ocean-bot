package linode

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Factory struct{}

func (Factory) Name() string { return "linode" }
func (Factory) Metadata() providers.Metadata {
	return providers.Metadata{WebsiteURL: "https://cloud.linode.com/", Name: "linode", DisplayName: "Linode", CredentialLabel: "Personal Access Token", Status: "ready", Defaults: providers.Defaults{
		Images: providers.ImagePolicy{Family: "ubuntu", Versions: []string{"26.04", "24.04", "22.04"}}, LifetimeMinMinutes: 90, LifetimeMaxMinutes: 120, DesiredServers: 5,
		BuildSpacingMinMinutes: 1, BuildSpacingMaxMinutes: 3, MaxConcurrent: 1, FallbackAnyRegion: true}}
}
func (Factory) Open(ctx context.Context, r providers.OpenRequest) (providers.Driver, error) {
	if strings.TrimSpace(r.AccountID) == "" || r.HTTPClient == nil || r.Credentials == nil {
		return nil, errors.New("Linode: invalid open request")
	}
	return &Driver{client: newClient(r.HTTPClient, r.Credentials), accountID: r.AccountID}, nil
}

type Driver struct {
	client    *client
	accountID string
}

func (d *Driver) Name() string { return "linode" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, Inventory: true, InlineSSHKeys: true}
}
func (d *Driver) Health(ctx context.Context) error { _, e := d.Account(ctx); return e }

type accountData struct {
	ID           string      `json:"euuid"`
	Email        string      `json:"email"`
	Capabilities []string    `json:"capabilities"`
	Balance      json.Number `json:"balance"`
	Uninvoiced   json.Number `json:"balance_uninvoiced"`
}

func (d *Driver) account(ctx context.Context) (accountData, error) {
	var x accountData
	if e := d.client.do(ctx, http.MethodGet, "/account", nil, &x); e != nil {
		return x, e
	}
	if strings.TrimSpace(x.ID) == "" {
		return x, problem(providers.ErrorUnavailable, "account", "MISSING_ACCOUNT_IDENTITY", 200)
	}
	if !contains(x.Capabilities, "Linodes") {
		return x, problem(providers.ErrorPermissionDenied, "account", "COMPUTE_NOT_AVAILABLE", 200)
	}
	return x, nil
}
func accountModel(x accountData) providers.Account {
	return providers.Account{ID: x.ID, Email: x.Email, Status: "active"}
}
func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	x, e := d.account(ctx)
	if e != nil {
		return providers.Account{}, e
	}
	return accountModel(x), nil
}

type serverData struct {
	ID      int64    `json:"id"`
	Label   string   `json:"label"`
	Status  string   `json:"status"`
	Region  string   `json:"region"`
	Type    string   `json:"type"`
	Created string   `json:"created"`
	IPv4    []string `json:"ipv4"`
	Tags    []string `json:"tags"`
}

func serverID(x serverData) string {
	if x.ID <= 0 {
		return ""
	}
	return strconv.FormatInt(x.ID, 10)
}
func validID(id string) bool {
	n, e := strconv.ParseInt(id, 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == id
}
func normalizeServer(x serverData) providers.Server {
	ip := ""
	for _, s := range x.IPv4 {
		p := publicIPv4(s)
		if p != "" {
			ip = p
			break
		}
	}
	state := providers.ServerStateUnknown
	switch x.Status {
	case "running":
		state = providers.ServerStateProvisioning
		if ip != "" {
			state = providers.ServerStateReady
		}
	case "offline", "stopped":
		state = providers.ServerStateStopped
	case "shutting_down":
		state = providers.ServerStateStopping
	case "deleting":
		state = providers.ServerStateDeleting
	case "booting", "busy", "rebooting", "provisioning", "migrating", "rebuilding", "cloning", "restoring":
		state = providers.ServerStateProvisioning
	}
	created, e := time.Parse(time.RFC3339, x.Created)
	if e != nil {
		created, _ = time.Parse("2006-01-02T15:04:05", x.Created)
	}
	tags := append([]string(nil), x.Tags...)
	for _, tag := range x.Tags {
		if strings.HasPrefix(tag, "dob-dep-") {
			tags = append(tags, "dob-deployment-"+strings.TrimPrefix(tag, "dob-dep-"))
		}
	}
	return providers.Server{ID: serverID(x), Name: x.Label, State: state, Ready: state == providers.ServerStateReady, RegionID: x.Region, PrimaryIPv4: ip, CreatedAt: created,
		Tags: tags, Metadata: map[string]any{"plan": x.Type, "provider_state": x.Status}}
}

// Restricted users may receive a permission-filtered list. That is not a complete
// account inventory and must never authorize disappearance or quota conclusions.
func (d *Driver) requireFullAccess(ctx context.Context) error {
	var profile struct {
		Restricted *bool `json:"restricted"`
	}
	if e := d.client.do(ctx, http.MethodGet, "/profile", nil, &profile); e != nil {
		// Only a 404 from the exact instance endpoint can prove absence.
		if providers.IsClass(e, providers.ErrorNotFound) {
			return problem(providers.ErrorUnavailable, "profile", "ACCESS_EVIDENCE_UNAVAILABLE", 404)
		}
		return e
	}
	if profile.Restricted == nil {
		return problem(providers.ErrorUnavailable, "profile", "MISSING_ACCESS_EVIDENCE", 200)
	}
	if *profile.Restricted {
		return problem(providers.ErrorPermissionDenied, "profile", "COMPLETE_INVENTORY_ACCESS_REQUIRED", 200)
	}
	return nil
}
func (d *Driver) rawServer(ctx context.Context, id string) (serverData, error) {
	var x serverData
	if !validID(id) {
		return x, problem(providers.ErrorInvalidRequest, "get_server", "INVALID_SERVER_ID", 0)
	}
	if e := d.requireFullAccess(ctx); e != nil {
		return x, e
	}
	if e := d.client.do(ctx, http.MethodGet, "/linode/instances/"+id, nil, &x); e != nil {
		return x, e
	}
	if serverID(x) != id || x.Status == "" || x.Region == "" {
		return x, problem(providers.ErrorUnavailable, "get_server", "INVALID_SERVER_RESPONSE", 200)
	}
	return x, nil
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	x, e := d.rawServer(ctx, id)
	if e != nil {
		return providers.Server{}, e
	}
	return normalizeServer(x), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	if e := d.requireFullAccess(ctx); e != nil {
		return nil, e
	}
	xs, e := list(ctx, d.client, "/linode/instances", serverID)
	if e != nil {
		return nil, e
	}
	out := make([]providers.Server, 0, len(xs))
	for _, x := range xs {
		if x.Status == "" || x.Region == "" {
			return nil, problem(providers.ErrorUnavailable, "list_servers", "INVALID_SERVER_RESPONSE", 200)
		}
		out = append(out, normalizeServer(x))
	}
	return out, nil
}
func (d *Driver) Inventory(ctx context.Context) (providers.Inventory, error) {
	ss, e := d.ListServers(ctx)
	if e != nil {
		return providers.Inventory{}, e
	}
	return providers.Inventory{Servers: ss, Resources: []providers.Resource{}, ObservedAt: time.Now().UTC()}, nil
}

// The published compute quota paths currently have no GET contract. Usage is
// authoritative; inventing a global server limit from usage or a vCPU quota is not.
func capacity(n int) providers.Capacity {
	return providers.Capacity{Source: "linode_api_usage_only", ComputeInUse: n, LimitKnown: false, ObservedAt: time.Now().UTC()}
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	if _, e := d.account(ctx); e != nil {
		return providers.Capacity{}, e
	}
	ss, e := d.ListServers(ctx)
	if e != nil {
		return providers.Capacity{}, e
	}
	return capacity(len(ss)), nil
}
func (d *Driver) observe(ctx context.Context, full bool) (providers.Observation, error) {
	a, e := d.account(ctx)
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
	return providers.Observation{Account: accountModel(a), Capacity: capacity(len(inv.Servers)), Inventory: inv, Catalog: cat, ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Observe(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, true)
}
func (d *Driver) ObserveFast(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, false)
}
func (d *Driver) Billing(ctx context.Context) (providers.Billing, error) {
	a, e := d.account(ctx)
	if e != nil {
		return providers.Billing{}, e
	}
	if providers.ValidateMoney(string(a.Balance)) != nil || providers.ValidateMoney(string(a.Uninvoiced)) != nil {
		return providers.Billing{}, problem(providers.ErrorUnavailable, "billing", "INVALID_BILLING_AMOUNT", 200)
	}
	return providers.Billing{Currency: "USD", Balance: string(a.Balance), BalanceLabel: "Account balance (positive: amount owed)", MonthToDate: string(a.Uninvoiced), PendingCharges: string(a.Uninvoiced), Invoices: []providers.Invoice{}, InvoiceStatusKnown: false, GeneratedAt: time.Now().UTC()}, nil
}
func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

var _ providers.Factory = Factory{}
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.CatalogReader = (*Driver)(nil)
var _ providers.InventoryReader = (*Driver)(nil)
var _ providers.ObservationReader = (*Driver)(nil)
var _ providers.FastObservationReader = (*Driver)(nil)
var _ providers.BillingReader = (*Driver)(nil)
