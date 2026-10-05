package upcloud

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
	"net/http"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

func validUUID(s string) bool { return uuidPattern.MatchString(s) }
func (d *Driver) CreateServer(ctx context.Context, r providers.CreateServerRequest) (providers.CreateServerResult, error) {
	reject := providers.CreateServerResult{Outcome: providers.OutcomeRejected}
	if r.Identity == "" || len(r.Identity) > 255 || r.Name == "" || r.RegionID == "" || !validUUID(r.ImageID) || len(r.SSHAuthorizedKeys) == 0 || len(r.SSHAuthorizedKeys) > 32 || len(r.SSHKeyRefs) > 0 {
		return reject, invalid("create_server", "identity, Ubuntu template and inline SSH key are required")
	}
	keys := make([]string, 0, len(r.SSHAuthorizedKeys))
	for _, key := range r.SSHAuthorizedKeys {
		key = strings.TrimSpace(key)
		_, _, options, rest, e := ssh.ParseAuthorizedKey([]byte(key))
		if e != nil || len(options) != 0 || len(rest) != 0 {
			return reject, invalid("create_server", "invalid SSH public key")
		}
		keys = append(keys, key)
	}
	ps, e := d.plans(ctx)
	if e != nil {
		return reject, e
	}
	var plan planData
	for _, p := range ps {
		if p.Name == r.PlanID {
			plan = p
			break
		}
	}
	if !usablePlan(plan) {
		return reject, invalid("create_server", "unsupported plan")
	}
	image, e := d.storage(ctx, r.ImageID)
	if e != nil {
		return reject, e
	}
	if !templateImage(image).Available || image.Size > plan.Disk {
		return reject, &providers.Error{Class: providers.ErrorImageUnavailable, Operation: "create_server", Message: "UpCloud: Ubuntu template unavailable or too large for plan"}
	}
	labels := []label{{"dob-owner", d.accountID}, {"dob-identity", r.Identity}}
	tags, _ := json.Marshal(r.Tags)
	if len(tags) <= 255 {
		labels = append(labels, label{"dob-tags", string(tags)})
	}
	iface := func(kind string) map[string]any {
		return map[string]any{"type": kind, "ip_addresses": map[string]any{"ip_address": []any{map[string]any{"family": "IPv4"}}}}
	}
	// Explicit IPv4 interfaces prevent default IPv6 allocation. Guest hardening
	// and Sanaei activation remain in the common provisioning path.
	body := map[string]any{"server": map[string]any{
		"zone": r.RegionID, "plan": r.PlanID, "title": r.Name, "hostname": r.Name,
		"labels":          map[string]any{"label": labels},
		"login_user":      map[string]any{"username": "root", "create_password": "no", "ssh_keys": map[string]any{"ssh_key": keys}},
		"networking":      map[string]any{"interfaces": map[string]any{"interface": []any{iface("public"), iface("utility")}}},
		"storage_devices": map[string]any{"storage_device": []any{map[string]any{"action": "clone", "storage": r.ImageID, "size": int(plan.Disk), "tier": plan.Tier, "title": r.Name + "-root", "address": "virtio:0", "type": "disk", "labels": labels}}},
		"firewall":        "off", // Shared guest provisioning manages service access, as for DO/Vultr.
		"metadata":        "yes", "simple_backup": "no", "remote_access_enabled": "no", "password_delivery": "none",
	}}
	var out struct {
		Server serverData `json:"server"`
	}
	e = d.client.do(ctx, http.MethodPost, "/server", body, &out)
	if e != nil {
		e = normalize("create_server", e)
		outcome := providers.OutcomeRejected
		if providers.IsRetryable(e) {
			outcome = providers.OutcomeAmbiguous
		}
		return providers.CreateServerResult{Outcome: outcome}, e
	}
	if !validUUID(out.Server.ID) {
		return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, &providers.Error{Class: providers.ErrorAmbiguousOutcome, Operation: "create_server", Message: "UpCloud create returned no valid server id"}
	}
	return providers.CreateServerResult{ServerID: out.Server.ID, Outcome: providers.OutcomeAccepted}, nil
}
func (d *Driver) storage(ctx context.Context, id string) (storageData, error) {
	if !validUUID(id) {
		return storageData{}, invalid("get_storage", "invalid storage UUID")
	}
	var out struct {
		Storage storageData `json:"storage"`
	}
	if e := d.client.do(ctx, http.MethodGet, "/storage/"+id, nil, &out); e != nil {
		return storageData{}, normalize("get_storage", e)
	}
	if out.Storage.ID != id {
		return storageData{}, unavailable("get_storage", "storage identity mismatch")
	}
	return out.Storage, nil
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	if !validUUID(id) {
		return invalid("delete_server", "invalid server UUID")
	}
	if d.cleanup == nil {
		return unavailable("delete_server", "durable cleanup journal unavailable")
	}
	manifest, e := d.cleanup.Load(ctx, d.accountID, id)
	if e != nil {
		return e
	}
	s, e := d.rawServer(ctx, id)
	if e != nil && !providers.IsClass(e, providers.ErrorNotFound) {
		return e
	}
	if e == nil {
		identity := labelValue(s.Labels.Items, "dob-identity")
		if identity == "" || labelValue(s.Labels.Items, "dob-owner") != d.accountID {
			return invalid("delete_server", "server ownership cannot be verified")
		}
		if manifest == nil {
			manifest = &providers.CleanupManifest{Identity: identity, StorageIDs: []string{}}
			for _, disk := range s.Disks.Items {
				if disk.Type == "cdrom" {
					continue
				}
				st, se := d.storage(ctx, disk.ID)
				if se != nil && !providers.IsClass(se, providers.ErrorNotFound) {
					return se
				}
				if se == nil && (labelValue(st.Labels, "dob-owner") != d.accountID || labelValue(st.Labels, "dob-identity") != identity) {
					return invalid("delete_server", "attached storage is not owned by this deployment")
				}
				manifest.StorageIDs = append(manifest.StorageIDs, disk.ID)
			}
			// Include owned disks detached before cleanup so they cannot leak charges.
			owned, se := list[storageData](ctx, d.client, "/storage/private", "storages", "storage")
			if se != nil {
				return normalize("delete_storage_inventory", se)
			}
			seen := map[string]bool{}
			for _, diskID := range manifest.StorageIDs {
				seen[diskID] = true
			}
			for _, st := range owned {
				if labelValue(st.Labels, "dob-owner") == d.accountID && labelValue(st.Labels, "dob-identity") == identity && st.Type != "backup" && st.Type != "template" {
					if !validUUID(st.ID) {
						return unavailable("delete_storage_inventory", "invalid owned storage identity")
					}
					if !seen[st.ID] {
						manifest.StorageIDs = append(manifest.StorageIDs, st.ID)
						seen[st.ID] = true
					}
				}
			}
			// Persist before stop/delete. Never replace a previous manifest on retry.
			if e = d.cleanup.Save(ctx, d.accountID, id, *manifest); e != nil {
				return e
			}
			manifest, e = d.cleanup.Load(ctx, d.accountID, id)
			if e != nil {
				return e
			}
		}
		if manifest == nil || manifest.Identity != identity || manifest.Complete {
			return invalid("delete_server", "cleanup manifest conflicts with server")
		}
		if s.State == "started" {
			// No polling loop in the worker hot path. Normal recovery re-reads state.
			e = d.client.do(ctx, http.MethodPost, "/server/"+id+"/stop", map[string]any{"stop_server": map[string]any{"stop_type": "soft", "timeout": "30"}}, nil)
			if e != nil {
				return normalize("stop_server", e)
			}
			return unavailable("delete_server", "stop requested; awaiting stopped state")
		}
		if s.State != "stopped" {
			return unavailable("delete_server", "waiting for stoppable server state")
		}
		// Detach first; delete only the recorded owned disks separately. A newly
		// attached manual disk must never be erased by a broad storages=1 request.
		if e = d.client.do(ctx, http.MethodDelete, "/server/"+id+"?storages=0", nil, nil); e != nil && !providers.IsClass(normalize("delete_server", e), providers.ErrorNotFound) {
			return normalize("delete_server", e)
		}
		if _, e = d.rawServer(ctx, id); e == nil {
			return unavailable("delete_server", "awaiting server deletion")
		} else if !providers.IsClass(e, providers.ErrorNotFound) {
			return e
		}
	}
	if manifest == nil || manifest.Complete {
		return nil
	}
	for _, storageID := range manifest.StorageIDs {
		st, se := d.storage(ctx, storageID)
		if providers.IsClass(se, providers.ErrorNotFound) {
			continue
		}
		if se != nil {
			return se
		}
		if labelValue(st.Labels, "dob-owner") != d.accountID || labelValue(st.Labels, "dob-identity") != manifest.Identity {
			return invalid("delete_storage", "storage ownership changed")
		}
		if len(st.Servers.Items) > 0 {
			return unavailable("delete_storage", "storage attached to a server")
		}
		if st.State != "online" {
			return unavailable("delete_storage", "storage not online")
		}
		// No automatic backups are created. Preserve any separately created backup.
		e = d.client.do(ctx, http.MethodDelete, "/storage/"+storageID+"?backups=keep", nil, nil)
		if e != nil && !providers.IsClass(normalize("delete_storage", e), providers.ErrorNotFound) {
			return normalize("delete_storage", e)
		}
		if _, e = d.storage(ctx, storageID); e == nil {
			return unavailable("delete_storage", "awaiting storage deletion")
		} else if !providers.IsClass(e, providers.ErrorNotFound) {
			return e
		}
	}
	return d.cleanup.Finish(ctx, d.accountID, id)
}
