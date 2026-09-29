package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type Driver struct{ client *Client }

type credentialBridge struct{ source providers.CredentialSource }

func (b credentialBridge) Get(ctx context.Context, _, _ string) ([]byte, error) {
	return b.source.Get(ctx)
}

type Factory struct{}

func (Factory) Name() string { return "digitalocean" }
func (Factory) Open(_ context.Context, req providers.OpenRequest) (providers.Driver, error) {
	if strings.TrimSpace(req.AccountID) == "" || req.HTTPClient == nil || req.Credentials == nil {
		return nil, fmt.Errorf("digitalocean driver: invalid open request")
	}
	c, err := NewClient(accounts.NewContext(req.AccountID), "provider-primary", credentialBridge{source: req.Credentials}, req.HTTPClient)
	if err != nil {
		return nil, err
	}
	return &Driver{client: c}, nil
}

func NewDriver(c *Client) (*Driver, error) {
	if c == nil {
		return nil, fmt.Errorf("digitalocean driver: nil client")
	}
	return &Driver{client: c}, nil
}

func (d *Driver) Name() string { return "digitalocean" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, SSHKeys: true, Inventory: true}
}
func (d *Driver) Health(ctx context.Context) error {
	_, err := d.client.GetAccount(ctx)
	return normalizeError("health", err)
}
func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	a, err := d.client.GetAccount(ctx)
	if err != nil {
		return providers.Account{}, normalizeError("account", err)
	}
	return providers.Account{ID: a.UUID, Email: a.Email, Status: a.Status}, nil
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	a, err := d.client.GetAccount(ctx)
	if err != nil {
		return providers.Capacity{}, normalizeError("capacity", err)
	}
	ds, err := d.client.ListDropletModels(ctx)
	if err != nil {
		return providers.Capacity{}, normalizeError("capacity", err)
	}
	return providers.Capacity{ComputeLimit: a.DropletLimit, ComputeInUse: len(ds), ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	raw, err := d.client.Catalog(ctx)
	if err != nil {
		return providers.Catalog{}, normalizeError("catalog", err)
	}
	return normalizeCatalog(raw), nil
}
func (d *Driver) CreateServer(ctx context.Context, req providers.CreateServerRequest) (providers.CreateServerResult, error) {
	keys := make([]any, 0, len(req.SSHKeyRefs))
	for _, id := range req.SSHKeyRefs {
		if n, err := strconv.Atoi(id); err == nil {
			keys = append(keys, n)
		} else {
			keys = append(keys, id)
		}
	}
	tags := append([]string(nil), req.Tags...)
	if req.Identity != "" && !contains(tags, req.Identity) {
		tags = append(tags, req.Identity)
	}
	raw, err := d.client.CreateDroplet(ctx, CreateDropletRequest{Name: req.Name, Region: req.RegionID, Size: req.PlanID, Image: req.ImageID, SSHKeys: keys, Tags: tags})
	if err != nil {
		pe := normalizeError("create_server", err)
		out := providers.OutcomeRejected
		if providers.IsRetryable(pe) {
			out = providers.OutcomeAmbiguous
		}
		return providers.CreateServerResult{Outcome: out}, pe
	}
	return providers.CreateServerResult{ServerID: strconv.Itoa(raw.ID), Outcome: providers.OutcomeAccepted}, nil
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	n, err := strconv.Atoi(id)
	if err != nil {
		return providers.Server{}, &providers.Error{Class: providers.ErrorInvalidRequest, Operation: "get_server", Message: "invalid digitalocean droplet id", Cause: err}
	}
	raw, err := d.client.GetDroplet(ctx, n)
	if err != nil {
		return providers.Server{}, normalizeError("get_server", err)
	}
	return normalizeServer(raw), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	raw, err := d.client.ListDropletModels(ctx)
	if err != nil {
		return nil, normalizeError("list_servers", err)
	}
	out := make([]providers.Server, 0, len(raw))
	for _, x := range raw {
		out = append(out, normalizeServer(x))
	}
	return out, nil
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	n, err := strconv.Atoi(id)
	if err != nil {
		return &providers.Error{Class: providers.ErrorInvalidRequest, Operation: "delete_server", Message: "invalid digitalocean droplet id", Cause: err}
	}
	return normalizeError("delete_server", d.client.DeleteDroplet(ctx, n))
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	all, err := d.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]providers.Server, 0, 1)
	for _, s := range all {
		if contains(s.Tags, identity) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (d *Driver) CreateSSHKey(ctx context.Context, name, publicKey string) (providers.SSHKey, error) {
	k, err := d.client.CreateSSHKey(ctx, name, publicKey)
	if err != nil {
		return providers.SSHKey{}, normalizeError("create_ssh_key", err)
	}
	return providers.SSHKey{ID: strconv.Itoa(k.ID), Name: k.Name, Fingerprint: k.Fingerprint}, nil
}
func (d *Driver) DeleteSSHKey(ctx context.Context, id string) error {
	n, err := strconv.Atoi(id)
	if err != nil {
		return &providers.Error{Class: providers.ErrorInvalidRequest, Operation: "delete_ssh_key", Message: "invalid digitalocean ssh key id", Cause: err}
	}
	return normalizeError("delete_ssh_key", d.client.DeleteSSHKey(ctx, n))
}
func (d *Driver) Inventory(ctx context.Context) (providers.Inventory, error) {
	servers, err := d.ListServers(ctx)
	if err != nil {
		return providers.Inventory{}, err
	}
	return providers.Inventory{Servers: servers, ObservedAt: time.Now().UTC()}, nil
}

func (d *Driver) Observe(ctx context.Context) (providers.Observation, error) {
	raw, err := d.client.Discover(ctx)
	if err != nil {
		return providers.Observation{}, normalizeError("observe", err)
	}
	now := time.Now().UTC()
	servers := make([]providers.Server, 0, len(raw.Droplets))
	for _, x := range raw.Droplets {
		servers = append(servers, normalizeServer(x))
	}
	return providers.Observation{
		Account:    providers.Account{ID: raw.Account.UUID, Email: raw.Account.Email, Status: raw.Account.Status},
		Capacity:   providers.Capacity{ComputeLimit: raw.Account.DropletLimit, ComputeInUse: len(raw.Droplets), ObservedAt: now},
		Catalog:    normalizeCatalog(raw),
		Inventory:  providers.Inventory{Servers: servers, ObservedAt: now},
		ObservedAt: now,
	}, nil
}

func normalizeCatalog(raw DiscoveryResult) providers.Catalog {
	out := providers.Catalog{Regions: make([]providers.Region, 0, len(raw.Regions)), Plans: make([]providers.Plan, 0, len(raw.Sizes)), Images: make([]providers.Image, 0, len(raw.Images))}
	for _, r := range raw.Regions {
		out.Regions = append(out.Regions, providers.Region{ID: r.Slug, Name: r.Name, Available: r.Available})
	}
	for _, s := range raw.Sizes {
		out.Plans = append(out.Plans, providers.Plan{ID: s.Slug, Name: s.Slug, CPU: s.VCPUs, MemoryMB: s.Memory, DiskGB: s.Disk, PriceHourly: s.PriceHourly, PriceMonthly: s.PriceMonthly, Available: s.Available, AvailableRegions: append([]string(nil), s.Regions...)})
	}
	for _, i := range raw.Images {
		out.Images = append(out.Images, normalizeImage(i))
	}
	return out
}
func normalizeImage(i Image) providers.Image {
	family := strings.ToLower(strings.TrimSpace(i.Distribution))
	version := ""
	name := strings.TrimSpace(i.Name)
	if fields := strings.Fields(name); len(fields) > 0 {
		version = fields[0]
	}
	arch := "x86_64"
	if strings.Contains(strings.ToLower(i.Slug), "arm") {
		arch = "arm64"
	}
	return providers.Image{ID: i.Slug, Name: i.Name, Family: family, Version: version, Architecture: arch, Available: i.Status == "available"}
}
func normalizeServer(d Droplet) providers.Server {
	state := providers.ServerStateUnknown
	switch strings.ToLower(d.Status) {
	case "new", "archive":
		state = providers.ServerStateProvisioning
	case "active":
		state = providers.ServerStateReady
	case "off":
		state = providers.ServerStateStopped
	}
	created, _ := time.Parse(time.RFC3339, d.CreatedAt)
	return providers.Server{ID: strconv.Itoa(d.ID), Name: d.Name, State: state, Ready: state == providers.ServerStateReady && d.PublicIPv4 != "", RegionID: d.Region.Slug, PrimaryIPv4: d.PublicIPv4, CreatedAt: created, Tags: append([]string(nil), d.Tags...)}
}
func normalizeError(op string, err error) error {
	if err == nil {
		return nil
	}
	var h HTTPError
	if errors.As(err, &h) {
		class := providers.ErrorUnknown
		switch {
		case h.Status == 401:
			class = providers.ErrorAuthentication
		case h.Status == 403 && strings.Contains(strings.ToLower(h.Message), "locked"):
			class = providers.ErrorAccountLocked
		case h.Status == 403:
			class = providers.ErrorPermissionDenied
		case h.Status == 404:
			class = providers.ErrorNotFound
		case h.Status == 429:
			class = providers.ErrorRateLimited
		case IsCapacityError(err):
			class = providers.ErrorRegionCapacity
		case h.Status == 400 || h.Status == 422:
			class = providers.ErrorInvalidRequest
		case h.Status == 408 || h.Status == 409 || h.Status >= 500:
			class = providers.ErrorUnavailable
		}
		return &providers.Error{Class: class, Operation: op, StatusCode: h.Status, RetryAfter: h.RetryAfter, Message: h.Message, Cause: err}
	}
	if errors.Is(err, ErrProviderRequest) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &providers.Error{Class: providers.ErrorTransport, Operation: op, Cause: err}
	}
	return &providers.Error{Class: providers.ErrorUnknown, Operation: op, Cause: err}
}
func contains(v []string, want string) bool {
	for _, x := range v {
		if x == want {
			return true
		}
	}
	return false
}

var _ providers.Driver = (*Driver)(nil)
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.CatalogReader = (*Driver)(nil)
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.SSHKeyDriver = (*Driver)(nil)
var _ providers.InventoryReader = (*Driver)(nil)
var _ providers.Observer = (*Driver)(nil)
var _ providers.Factory = Factory{}
