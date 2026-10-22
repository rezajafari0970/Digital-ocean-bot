package ovh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
)

type Factory struct{}

func (Factory) Name() string { return "ovh" }
func (Factory) Metadata() providers.Metadata {
	return providers.Metadata{Name: "ovh", DisplayName: "OVH Public Cloud", CredentialLabel: "OVH credentials", CredentialFields: []providers.CredentialField{{Key: "endpoint", Label: "Endpoint: ovh-eu / ovh-ca / ovh-us", Default: "ovh-eu"}, {Key: "application_key", Label: "Application key", Secret: true}, {Key: "application_secret", Label: "Application secret", Secret: true}, {Key: "consumer_key", Label: "Consumer key", Secret: true}, {Key: "project_id", Label: "Public Cloud project ID"}}, CredentialHelp: `JSON: {"endpoint":"ovh-eu","application_key":"…","application_secret":"…","consumer_key":"…","project_id":"…"}. Endpoints: ovh-eu, ovh-ca, ovh-us. Grant GET on the selected project and its children, POST on /instance, DELETE on /instance/*. Uses hourly x86 public-cloud instances with local disks in standard 1-AZ regions. 3-AZ and Local Zones are not offered.`, Status: "ready", Defaults: cloudutil.Defaults()}
}
func (Factory) Open(ctx context.Context, r providers.OpenRequest) (providers.Driver, error) {
	if strings.TrimSpace(r.AccountID) == "" || r.HTTPClient == nil || r.Credentials == nil {
		return nil, errors.New("OVH: invalid open request")
	}
	cred, e := credentials(ctx, r.Credentials)
	if e != nil {
		return nil, e
	}
	if r.ExpectedExternalID != "" && r.ExpectedExternalID != cred.Endpoint+":"+cred.Project {
		return nil, problem(providers.ErrorAuthentication, "account", "PROJECT_SCOPE_MISMATCH", 0)
	}
	return &Driver{client: &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: r.Credentials, endpoint: cred.Endpoint, project: cred.Project}, accountID: r.AccountID, absence: r.Absence}, nil
}

type Driver struct {
	client    *client
	accountID string
	absence   providers.AbsenceVerifier
}

func (d *Driver) Name() string { return "ovh" }
func (d *Driver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Catalog: true, Compute: true, Inventory: true, InlineSSHKeys: true}
}
func (d *Driver) Health(ctx context.Context) error { _, e := d.Account(ctx); return e }
func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	var x struct {
		ID     string `json:"project_id"`
		Status string `json:"status"`
	}
	if e := d.client.do(ctx, http.MethodGet, d.client.path(""), nil, &x); e != nil {
		return providers.Account{}, e
	}
	if x.ID != d.client.project {
		return providers.Account{}, problem(providers.ErrorUnavailable, "account", "PROJECT_ID_MISMATCH", 200)
	}
	status := "unknown"
	switch x.Status {
	case "ok":
		status = "active"
	case "suspended":
		status = "locked"
	case "creating", "deleting", "deleted":
		status = "disabled"
	default:
		return providers.Account{}, problem(providers.ErrorUnavailable, "account", "UNKNOWN_PROJECT_STATUS", 200)
	}
	return providers.Account{ID: d.client.endpoint + ":" + x.ID, Status: status}, nil
}

type serverData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Region  string `json:"region"`
	Status  string `json:"status"`
	Created string `json:"created"`
	Flavor  string `json:"flavorId"`
	Image   string `json:"imageId"`
	IPs     []struct {
		IP      string `json:"ip"`
		Type    string `json:"type"`
		Version int    `json:"version"`
	} `json:"ipAddresses"`
	Monthly any `json:"monthlyBilling"`
}

func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:16]) }
func (d *Driver) name(identity string) string {
	return "dob-" + digest("ovh-owner-v1\x00"+d.accountID) + "-" + digest("ovh-deployment-v1\x00"+identity)
}
func (d *Driver) owns(x serverData) bool {
	prefix := "dob-" + digest("ovh-owner-v1\x00"+d.accountID) + "-"
	if !strings.HasPrefix(x.Name, prefix) || len(x.Name) != len(prefix)+32 {
		return false
	}
	_, e := hex.DecodeString(strings.TrimPrefix(x.Name, prefix))
	return e == nil
}
func (d *Driver) normalize(x serverData) providers.Server {
	ip := ""
	for _, a := range x.IPs {
		if a.Type == "public" && a.Version == 4 {
			if v := cloudutil.PublicIPv4(a.IP); v != "" {
				ip = v
				break
			}
		}
	}
	state := providers.ServerStateUnknown
	switch x.Status {
	case "ACTIVE":
		state = providers.ServerStateProvisioning
		if ip != "" {
			state = providers.ServerStateReady
		}
	case "BUILD", "BUILDING", "REBOOT", "HARD_REBOOT", "REBUILD", "RESUMING", "MIGRATING":
		state = providers.ServerStateProvisioning
	case "SHUTOFF", "STOPPED", "SUSPENDED", "PAUSED", "SHELVED", "SHELVED_OFFLOADED":
		state = providers.ServerStateStopped
	case "DELETING", "DELETED", "SOFT_DELETED":
		state = providers.ServerStateDeleting
	}
	created, _ := time.Parse(time.RFC3339, x.Created)
	tags := []string{}
	if d.owns(x) {
		tags = []string{"managed-by-digital-ocean-bot", "dob-account-" + d.accountID}
	}
	return providers.Server{ID: x.ID, Name: x.Name, State: state, Ready: state == providers.ServerStateReady, RegionID: x.Region, PrimaryIPv4: ip, CreatedAt: created, Tags: tags, Metadata: map[string]any{"plan": x.Flavor, "image": x.Image, "provider_state": x.Status}}
}
func validServer(x serverData) bool {
	return uuidPattern.MatchString(x.ID) && regionPattern.MatchString(x.Region) && x.Status != "" && x.Name != ""
}
func (d *Driver) raw(ctx context.Context, id string) (serverData, error) {
	var x serverData
	if !uuidPattern.MatchString(id) {
		return x, problem(providers.ErrorInvalidRequest, "get_server", "INVALID_SERVER_ID", 0)
	}
	if e := d.client.do(ctx, http.MethodGet, d.client.path("/instance/"+id), nil, &x); e != nil {
		if providers.IsClass(e, providers.ErrorNotFound) {
			if pending := providers.ConfirmAbsence(ctx, d.absence, id); pending != nil {
				return x, pending
			}
		}
		return x, e
	}
	if x.ID != id || !validServer(x) {
		return x, problem(providers.ErrorUnavailable, "get_server", "INVALID_SERVER_RESPONSE", 200)
	}
	if d.absence != nil {
		if e := d.absence.Present(ctx, id); e != nil {
			return x, e
		}
	}
	return x, nil
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	x, e := d.raw(ctx, id)
	if e != nil {
		return providers.Server{}, e
	}
	if x.Status == "DELETED" || x.Status == "SOFT_DELETED" {
		return providers.Server{}, problem(providers.ErrorNotFound, "get_server", "INSTANCE_DELETED", 404)
	}
	return d.normalize(x), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	xs, e := list(ctx, d.client, d.client.path("/instance"), func(x serverData) string { return x.ID })
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, x := range xs {
		if !validServer(x) {
			return nil, problem(providers.ErrorUnavailable, "inventory", "INVALID_SERVER", 200)
		}
		if x.Status != "DELETED" && x.Status != "SOFT_DELETED" {
			if d.absence != nil {
				if e := d.absence.Present(ctx, x.ID); e != nil {
					return nil, e
				}
			}
			out = append(out, d.normalize(x))
		}
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
func capacity(n int) providers.Capacity {
	return providers.Capacity{Source: "ovh_project_inventory_regional_quotas", ComputeInUse: n, LimitKnown: false, ObservedAt: time.Now().UTC()}
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	inv, e := d.Inventory(ctx)
	if e != nil {
		return providers.Capacity{}, e
	}
	return capacity(len(inv.Servers)), nil
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
	return providers.Observation{Account: a, Inventory: inv, Capacity: capacity(len(inv.Servers)), Catalog: cat, ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Observe(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, true)
}
func (d *Driver) ObserveFast(ctx context.Context) (providers.Observation, error) {
	return d.observe(ctx, false)
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	if identity == "" {
		return nil, problem(providers.ErrorInvalidRequest, "find_server", "MISSING_IDENTITY", 0)
	}
	xs, e := d.ListServers(ctx)
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, x := range xs {
		if x.Name == d.name(identity) {
			x.Tags = append(x.Tags, identity)
			out = append(out, x)
		}
	}
	return out, nil
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	x, e := d.raw(ctx, id)
	if providers.IsClass(e, providers.ErrorNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if !d.owns(x) {
		return problem(providers.ErrorPermissionDenied, "delete_server", "OWNERSHIP_NOT_PROVEN", 0)
	}
	e = d.client.do(ctx, http.MethodDelete, d.client.path("/instance/"+id), nil, nil)
	if providers.IsClass(e, providers.ErrorNotFound) {
		return nil
	}
	return e
}
func (d *Driver) CreateServer(ctx context.Context, r providers.CreateServerRequest) (providers.CreateServerResult, error) {
	rejected := providers.CreateServerResult{Outcome: providers.OutcomeRejected}
	invalid := func(code string) (providers.CreateServerResult, error) {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", code, 0)
	}
	if r.Identity == "" || len(r.Identity) > 256 || strings.ContainsAny(r.Identity, "\x00\r\n") || !regionPattern.MatchString(r.RegionID) || !uuidPattern.MatchString(r.PlanID) || !uuidPattern.MatchString(r.ImageID) || len(r.SSHKeyRefs) > 0 {
		return invalid("INVALID_CREATE_REQUEST")
	}
	data, e := cloudutil.CloudInit(r.SSHAuthorizedKeys)
	if e != nil {
		return invalid("INVALID_SSH_KEYS")
	}
	region, e := d.region(ctx, r.RegionID)
	if e != nil {
		return rejected, e
	}
	if !region.supported() {
		return invalid("REGION_DEPLOYMENT_MODE_UNSUPPORTED")
	}
	// Exact immutable native IDs come from the persisted regional launch snapshot.
	fs, e := d.flavors(ctx, r.RegionID)
	if e != nil {
		return rejected, e
	}
	var flavor flavorData
	for _, f := range fs {
		if f.ID == r.PlanID && usableFlavor(f) && f.Region == r.RegionID {
			flavor = f
			break
		}
	}
	if flavor.ID == "" {
		return invalid("FLAVOR_UNAVAILABLE")
	}
	ims, e := d.images(ctx, r.RegionID)
	if e != nil {
		return rejected, e
	}
	matched := false
	for _, im := range ims {
		if im.ID == r.ImageID && usableImage(im) && im.Region == r.RegionID && im.MinDisk <= flavor.Disk && im.MinRAM <= flavor.RAM*1024 && (im.FlavorType == "" || im.FlavorType == flavor.Type) {
			matched = true
		}
	}
	if !matched {
		return invalid("IMAGE_REGION_OR_FLAVOR_MISMATCH")
	}
	if e = d.checkQuota(ctx, r.RegionID, flavor); e != nil {
		return rejected, e
	}
	body := map[string]any{"name": d.name(r.Identity), "region": r.RegionID, "flavorId": r.PlanID, "imageId": r.ImageID, "monthlyBilling": false, "userData": data}
	var x serverData
	if e = d.client.do(ctx, http.MethodPost, d.client.path("/instance"), body, &x); e != nil {
		if cloudutil.Ambiguous(e) {
			return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_UNCONFIRMED", 0)
		}
		return rejected, e
	}
	if !uuidPattern.MatchString(x.ID) || x.Name != d.name(r.Identity) || x.Region != r.RegionID || x.Monthly != nil {
		return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_RESPONSE_UNCONFIRMED", 200)
	}
	return providers.CreateServerResult{ServerID: x.ID, Outcome: providers.OutcomeAccepted}, nil
}

var _ providers.Factory = Factory{}
var _ providers.ComputeDriver = (*Driver)(nil)
var _ providers.AccountReader = (*Driver)(nil)
var _ providers.CatalogReader = (*Driver)(nil)
var _ providers.InventoryReader = (*Driver)(nil)
var _ providers.ObservationReader = (*Driver)(nil)
var _ providers.FastObservationReader = (*Driver)(nil)
