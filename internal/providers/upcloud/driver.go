package upcloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Factory struct{}

func (Factory) Name() string { return "upcloud" }
func (Factory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "upcloud", DisplayName: "UpCloud", CredentialLabel: "API token (ucat_…)", Status: "ready", Defaults: providers.Defaults{
		Images: providers.ImagePolicy{Family: "ubuntu", Versions: []string{"26.04", "24.04", "22.04"}}, LifetimeMinMinutes: 90, LifetimeMaxMinutes: 120, DesiredServers: 5, BuildSpacingMinMinutes: 1, BuildSpacingMaxMinutes: 3, MaxConcurrent: 1, FallbackAnyRegion: true}}
}
func (Factory) Open(ctx context.Context, r providers.OpenRequest) (providers.Driver, error) {
	if strings.TrimSpace(r.AccountID) == "" || r.HTTPClient == nil || r.Credentials == nil {
		return nil, errors.New("UpCloud: invalid open request")
	}
	return &Driver{client: newClient(r.HTTPClient, r.Credentials), accountID: r.AccountID, planIDs: append([]string(nil), r.PlanIDs...), cleanup: r.Cleanup}, nil
}

type Driver struct {
	client    *Client
	accountID string
	planIDs   []string
	cleanup   providers.CleanupJournal
}

func (d *Driver) Name() string { return "upcloud" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, Inventory: true, InlineSSHKeys: true}
}
func (d *Driver) Health(ctx context.Context) error { _, e := d.Account(ctx); return e }
func (d *Driver) account(ctx context.Context) (accountData, error) {
	var x struct {
		Account accountData `json:"account"`
	}
	e := d.client.do(ctx, http.MethodGet, "/account", nil, &x)
	if e != nil {
		return accountData{}, normalize("account", e)
	}
	if x.Account.Username == "" {
		return accountData{}, unavailable("account", "missing account identity")
	}
	return x.Account, nil
}
func accountModel(a accountData) providers.Account {
	sum := sha256.Sum256([]byte("upcloud-account:" + a.Username))
	return providers.Account{ID: "upcloud-" + hex.EncodeToString(sum[:16]), Status: "active"}
}
func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	a, e := d.account(ctx)
	if e != nil {
		return providers.Account{}, e
	}
	return accountModel(a), nil
}
func unavailable(op, msg string) error {
	return &providers.Error{Class: providers.ErrorUnavailable, Operation: op, Message: "UpCloud: " + msg}
}
func invalid(op, msg string) error {
	return &providers.Error{Class: providers.ErrorInvalidRequest, Operation: op, Message: "UpCloud: " + msg}
}

// An absent/null list is not a successful empty inventory.
func list[T any](ctx context.Context, c *Client, path, outer, inner string) ([]T, error) {
	var envelope map[string]json.RawMessage
	if e := c.do(ctx, http.MethodGet, path, nil, &envelope); e != nil {
		return nil, e
	}
	var nested map[string]json.RawMessage
	if e := json.Unmarshal(envelope[outer], &nested); e != nil {
		return nil, &responseError{Code: "MISSING_LIST_ENVELOPE", Status: http.StatusOK}
	}
	b := nested[inner]
	if len(b) == 0 || strings.TrimSpace(string(b)) == "null" {
		return nil, &responseError{Code: "MISSING_LIST", Status: http.StatusOK}
	}
	var out []T
	if e := json.Unmarshal(b, &out); e != nil || out == nil {
		return nil, &responseError{Code: "INVALID_LIST", Status: http.StatusOK}
	}
	return out, nil
}
func (d *Driver) plans(ctx context.Context) ([]planData, error) {
	xs, e := list[planData](ctx, d.client, "/plan", "plans", "plan")
	if e != nil {
		return nil, normalize("catalog_plans", e)
	}
	seen := map[string]bool{}
	for _, p := range xs {
		if p.Name == "" || seen[p.Name] {
			return nil, unavailable("catalog_plans", "invalid or duplicate plan")
		}
		seen[p.Name] = true
	}
	return xs, nil
}
func usablePlan(p planData) bool {
	return p.Name != "" && p.CPU > 0 && p.Memory > 0 && p.Disk > 0 && p.GPU == 0 && (p.Tier == "maxiops" || p.Tier == "standard" || p.Tier == "hdd")
}
func templateImage(x storageData) providers.Image {
	name := strings.ToLower(x.Title)
	ver := ""
	if strings.Contains(name, "ubuntu") && !strings.Contains(name, "arm") {
		for _, v := range []string{"26.04", "24.04", "22.04"} {
			if strings.Contains(name, v) {
				ver = v
				break
			}
		}
	}
	return providers.Image{ID: x.ID, Name: x.Title, Family: "ubuntu", Version: ver, Architecture: "x86_64", Available: ver != "" && x.ID != "" && x.Type == "template" && x.Access == "public" && x.State == "online"}
}
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	type zone struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Public      string `json:"public"`
	}
	zs, e := list[zone](ctx, d.client, "/zone", "zones", "zone")
	if e != nil {
		return providers.Catalog{}, normalize("catalog_zones", e)
	}
	ps, e := d.plans(ctx)
	if e != nil {
		return providers.Catalog{}, e
	}
	ts, e := list[storageData](ctx, d.client, "/storage/template", "storages", "storage")
	if e != nil {
		return providers.Catalog{}, normalize("catalog_templates", e)
	}
	out := providers.Catalog{Regions: []providers.Region{}, Plans: []providers.Plan{}, Images: []providers.Image{}}
	regions := []string{}
	for _, z := range zs {
		if z.Public == "yes" && z.ID != "" {
			out.Regions = append(out.Regions, providers.Region{ID: z.ID, Name: z.Description, Available: true})
			regions = append(regions, z.ID)
		}
	}
	// Sort by resource footprint. Prices vary by zone/currency; never display zero as free.
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].CPU != ps[j].CPU {
			return ps[i].CPU < ps[j].CPU
		}
		if ps[i].Memory != ps[j].Memory {
			return ps[i].Memory < ps[j].Memory
		}
		return ps[i].Name < ps[j].Name
	})
	for _, p := range ps {
		if usablePlan(p) {
			out.Plans = append(out.Plans, providers.Plan{ID: p.Name, Name: p.Name, CPU: int(p.CPU), MemoryMB: int(p.Memory), DiskGB: int(p.Disk), Available: len(regions) > 0, AvailableRegions: append([]string(nil), regions...)})
		}
	}
	for _, t := range ts {
		im := templateImage(t)
		if im.Available {
			out.Images = append(out.Images, im)
		}
	}
	if len(out.Regions) == 0 || len(out.Plans) == 0 || len(out.Images) == 0 {
		return providers.Catalog{}, withDiagnostic(unavailable("catalog", "no compatible public zones, plans or Ubuntu templates"), fmt.Sprintf("NO_CATALOG_z%d_p%d_i%d", len(out.Regions), len(out.Plans), len(out.Images)))
	}
	return out, nil
}
func (d *Driver) rawServers(ctx context.Context) ([]serverData, error) {
	out := []serverData{}
	seen := map[string]bool{}
	for offset := 0; offset < 20000; {
		xs, e := list[serverData](ctx, d.client, fmt.Sprintf("/server?limit=100&offset=%d&order_by=title&sort_by=asc", offset), "servers", "server")
		if e != nil {
			return nil, normalize("list_servers", e)
		}
		if len(xs) == 0 {
			return out, nil
		}
		for _, x := range xs {
			if x.ID == "" || seen[x.ID] {
				return nil, unavailable("list_servers", "duplicate or missing server id; incomplete inventory")
			}
			seen[x.ID] = true
			out = append(out, x)
		}
		offset += len(xs)
	}
	return nil, unavailable("list_servers", "inventory exceeds pagination bound")
}
func normalizeServer(x serverData) providers.Server {
	var ip string
	for _, v := range x.IPs.Items {
		p := net.ParseIP(v.Address)
		if v.Access == "public" && v.Family == "IPv4" && p != nil && p.To4() != nil && !p.IsUnspecified() && !p.IsLoopback() && !p.IsPrivate() {
			ip = v.Address
			break
		}
	}
	state := providers.ServerStateUnknown
	switch x.State {
	case "started":
		state = providers.ServerStateProvisioning
		if ip != "" {
			state = providers.ServerStateReady
		}
	case "stopped":
		state = providers.ServerStateStopped
	case "maintenance":
		state = providers.ServerStateProvisioning
	}
	var created time.Time
	if len(x.Created) > 0 {
		s := strings.Trim(string(x.Created), "\"")
		if n, e := strconv.ParseInt(s, 10, 64); e == nil {
			created = time.Unix(n, 0).UTC()
		} else {
			created, _ = time.Parse(time.RFC3339, s)
		}
	}
	tags := append([]string(nil), x.Tags.Items...)
	var storedTags []string
	if json.Unmarshal([]byte(labelValue(x.Labels.Items, "dob-tags")), &storedTags) == nil {
		tags = append(tags, storedTags...)
	}
	if identity := labelValue(x.Labels.Items, "dob-identity"); identity != "" {
		tags = append(tags, identity)
	}
	return providers.Server{ID: x.ID, Name: x.Title, State: state, Ready: state == providers.ServerStateReady, RegionID: x.Zone, PrimaryIPv4: ip, CreatedAt: created, Tags: tags, Metadata: map[string]any{"plan": x.Plan, "provider_state": x.State, "owner": labelValue(x.Labels.Items, "dob-owner"), "identity": labelValue(x.Labels.Items, "dob-identity")}}
}
func (d *Driver) rawServer(ctx context.Context, id string) (serverData, error) {
	if !validUUID(id) {
		return serverData{}, invalid("get_server", "invalid server UUID")
	}
	var x struct {
		Server serverData `json:"server"`
	}
	e := d.client.do(ctx, http.MethodGet, "/server/"+id, nil, &x)
	if e != nil {
		return serverData{}, normalize("get_server", e)
	}
	if x.Server.ID != id {
		return serverData{}, unavailable("get_server", "server identity mismatch")
	}
	return x.Server, nil
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	x, e := d.rawServer(ctx, id)
	if providers.IsClass(e, providers.ErrorNotFound) && d.cleanup != nil {
		m, je := d.cleanup.Load(ctx, d.accountID, id)
		if je != nil {
			return providers.Server{}, je
		}
		if m != nil && !m.Complete {
			return providers.Server{ID: id, State: providers.ServerStateDeleting, Metadata: map[string]any{"cleanup_pending": true}}, nil
		}
	}
	if e != nil {
		return providers.Server{}, e
	}
	return normalizeServer(x), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	xs, e := d.rawServers(ctx)
	if e != nil {
		return nil, e
	}
	ips, e := list[ipData](ctx, d.client, "/ip_address", "ip_addresses", "ip_address")
	if e != nil {
		return nil, normalize("inventory_ips", e)
	}
	byServer := map[string][]ipData{}
	for _, ip := range ips {
		byServer[ip.Server] = append(byServer[ip.Server], ip)
	}
	out := make([]providers.Server, 0, len(xs))
	for _, s := range xs {
		s.IPs.Items = byServer[s.ID]
		out = append(out, normalizeServer(s))
	}
	return out, nil
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	if identity == "" {
		return nil, invalid("find_server", "empty deployment identity")
	}
	xs, e := d.rawServers(ctx)
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, s := range xs {
		if labelValue(s.Labels.Items, "dob-owner") == d.accountID && labelValue(s.Labels.Items, "dob-identity") == identity {
			out = append(out, normalizeServer(s))
		}
	}
	return out, nil
}
func (d *Driver) Inventory(ctx context.Context) (providers.Inventory, error) {
	xs, e := d.ListServers(ctx)
	if e != nil {
		return providers.Inventory{}, e
	}
	disks, e := list[storageData](ctx, d.client, "/storage/private", "storages", "storage")
	if e != nil {
		return providers.Inventory{}, normalize("inventory_storages", e)
	}
	rs := []providers.Resource{}
	for _, x := range disks {
		if x.ID == "" {
			return providers.Inventory{}, unavailable("inventory_storages", "missing storage identity")
		}
		rs = append(rs, providers.Resource{ID: x.ID, Type: "storage", State: x.State, Managed: x.Type == "normal" && labelValue(x.Labels, "dob-owner") == d.accountID && labelValue(x.Labels, "dob-identity") != "", Metadata: map[string]any{"name": x.Title, "size_gib": int64(x.Size), "identity": labelValue(x.Labels, "dob-identity"), "tier": x.Tier}})
	}
	return providers.Inventory{Servers: xs, Resources: rs, ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) capacity(ctx context.Context, a accountData, inUse int) (providers.Capacity, error) {
	out := providers.Capacity{ComputeInUse: inUse, ObservedAt: time.Now().UTC(), Source: "upcloud_resource_budget", PlanAvailable: map[string]int{}}
	if len(d.planIDs) == 0 {
		return out, nil
	} // Preview has not selected plans yet.
	var usage resourceLimits
	if e := d.client.do(ctx, http.MethodGet, "/account/resource-usage", nil, &usage); e != nil {
		return out, normalize("capacity_usage", e)
	}
	plans, e := d.plans(ctx)
	if e != nil {
		return out, e
	}
	byID := map[string]planData{}
	for _, p := range plans {
		byID[p.Name] = p
	}
	remaining := int64(1 << 30)
	for _, id := range d.planIDs {
		p, ok := byID[id]
		if !ok || !usablePlan(p) {
			return out, invalid("capacity", "configured plan is unavailable")
		}
		// Current OpenAPI defines memory in MiB, storage quotas/plan sizes in GiB.
		needs := map[string]number{"cores": p.CPU, "memory": p.Memory, "public_ipv4": 1, "storage_total": p.Disk, "storage_" + p.Tier: p.Disk}
		special := "cloud_server_" + strings.ReplaceAll(strings.ToLower(id), "-", "_") + "_plans"
		if _, ok := a.Limits[special]; ok {
			needs[special] = 1
		}
		slots := int64(1 << 30)
		for k, per := range needs {
			limit, lok := a.Limits[k]
			used, uok := usage[k]
			if lok && limit == nil {
				return out, normalize("capacity_limits", &responseError{Code: "UNKNOWN_PLAN_QUOTA", Status: http.StatusOK})
			}
			if uok && used == nil {
				return out, normalize("capacity_usage", &responseError{Code: "UNKNOWN_RESOURCE_USAGE", Status: http.StatusOK})
			}
			if !lok || !uok || per <= 0 {
				return out, unavailable("capacity", "missing resource quota or usage: "+k)
			}
			slots = min(slots, max(int64(0), int64(*limit-*used))/int64(per))
		}
		out.PlanAvailable[id] = int(slots)
		remaining = min(remaining, slots)
	}
	out.LimitKnown = true
	out.ComputeLimit = inUse + int(remaining)
	return out, nil
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	a, e := d.account(ctx)
	if e != nil {
		return providers.Capacity{}, e
	}
	ss, e := d.rawServers(ctx)
	if e != nil {
		return providers.Capacity{}, e
	}
	return d.capacity(ctx, a, len(ss))
}
func (d *Driver) observe(ctx context.Context, withCatalog bool) (providers.Observation, error) {
	a, e := d.account(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	inv, e := d.Inventory(ctx)
	if e != nil {
		return providers.Observation{}, e
	}
	cap, e := d.capacity(ctx, a, len(inv.Servers))
	if e != nil {
		return providers.Observation{}, e
	}
	cat := providers.Catalog{}
	if withCatalog {
		cat, e = d.Catalog(ctx)
		if e != nil {
			return providers.Observation{}, e
		}
	}
	return providers.Observation{Account: accountModel(a), Inventory: inv, Capacity: cap, Catalog: cat, ObservedAt: time.Now().UTC()}, nil
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
	if e = providers.ValidateMoney(string(a.Credits)); e != nil {
		return providers.Billing{}, e
	}
	var billing struct {
		Currency string      `json:"currency"`
		Total    json.Number `json:"total_amount"`
	}
	if e = d.client.do(ctx, http.MethodGet, "/account/billing/summary/"+time.Now().UTC().Format("2006-01"), nil, &billing); e != nil {
		return providers.Billing{}, normalize("billing_summary", e)
	}
	if len(billing.Currency) != 3 {
		return providers.Billing{}, unavailable("billing", "missing billing currency")
	}
	if e = providers.ValidateMoney(string(billing.Total)); e != nil {
		return providers.Billing{}, e
	}
	return providers.Billing{Currency: billing.Currency, Balance: string(a.Credits), BalanceLabel: "Available credit", MonthToDate: string(billing.Total), Invoices: []providers.Invoice{}, InvoiceStatusKnown: false, GeneratedAt: time.Now().UTC()}, nil
}

var _ providers.Factory = Factory{}
var _ providers.Driver = (*Driver)(nil)
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.CatalogReader = (*Driver)(nil)
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.InventoryReader = (*Driver)(nil)
var _ providers.ObservationReader = (*Driver)(nil)
var _ providers.FastObservationReader = (*Driver)(nil)
var _ providers.BillingReader = (*Driver)(nil)
