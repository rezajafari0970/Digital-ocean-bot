package vultr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func (d *Driver) Account(ctx context.Context) (providers.Account, error) {
	x, err := d.client.Account(ctx)
	if err != nil {
		return providers.Account{}, normalizeError("account", err)
	}
	email := strings.ToLower(strings.TrimSpace(x.Account.Email))
	if email == "" {
		return providers.Account{}, &providers.Error{Class: providers.ErrorUnavailable, Message: "vultr account email missing"}
	}
	sum := sha256.Sum256([]byte("vultr-account:" + email))
	return providers.Account{ID: "vultr-" + hex.EncodeToString(sum[:16]), Email: x.Account.Email, Status: "active"}, nil
}
func (d *Driver) Capacity(ctx context.Context) (providers.Capacity, error) {
	xs, err := d.client.Instances(ctx)
	if err != nil {
		return providers.Capacity{}, normalizeError("capacity", err)
	}
	return providers.Capacity{LimitKnown: false, ComputeInUse: len(xs), ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	rs, err := d.client.Regions(ctx)
	if err != nil {
		return providers.Catalog{}, normalizeError("catalog_regions", err)
	}
	ps, err := d.client.Plans(ctx)
	if err != nil {
		return providers.Catalog{}, normalizeError("catalog_plans", err)
	}
	oses, err := d.client.OS(ctx)
	if err != nil {
		return providers.Catalog{}, normalizeError("catalog_os", err)
	}
	out := providers.Catalog{}
	availableRegions := map[string]bool{}
	for _, p := range ps {
		for _, rid := range p.Locations {
			availableRegions[rid] = true
		}
	}
	for _, r := range rs {
		out.Regions = append(out.Regions, providers.Region{ID: r.ID, Name: strings.TrimSpace(r.City + ", " + r.Country), Available: availableRegions[r.ID]})
	}
	for _, p := range ps {
		out.Plans = append(out.Plans, providers.Plan{ID: p.ID, Name: p.ID, CPU: p.VCPUCount, MemoryMB: p.RAM, DiskGB: p.Disk, PriceMonthly: p.MonthlyCost, Available: len(p.Locations) > 0, AvailableRegions: append([]string(nil), p.Locations...)})
	}
	for _, o := range oses {
		fam, ver := normalizeOS(o)
		out.Images = append(out.Images, providers.Image{ID: strconv.Itoa(o.ID), Name: o.Name, Family: fam, Version: ver, Architecture: o.Arch, Available: true})
	}
	return out, nil
}
func normalizeOS(o osItem) (string, string) {
	name := strings.ToLower(o.Name)
	family := strings.ToLower(strings.TrimSpace(o.Family))
	if family == "" && strings.Contains(name, "ubuntu") {
		family = "ubuntu"
	}
	ver := ""
	if family == "ubuntu" {
		for _, v := range []string{"26.04", "24.04", "22.04"} {
			if strings.Contains(name, v) {
				ver = v
				break
			}
		}
	}
	return family, ver
}
func normalizeServer(x instance) providers.Server {
	if ip := net.ParseIP(strings.TrimSpace(x.MainIP)); ip == nil || ip.To4() == nil {
		x.MainIP = ""
	}
	state := providers.ServerStateUnknown
	ready := false
	switch strings.ToLower(x.Status) {
	case "pending", "installing":
		state = providers.ServerStateProvisioning
	case "active":
		serverOK := x.ServerStatus == "" || strings.EqualFold(x.ServerStatus, "ok")
		powerOK := x.PowerStatus == "" || strings.EqualFold(x.PowerStatus, "running")
		ready = strings.TrimSpace(x.MainIP) != "" && x.MainIP != "0.0.0.0" && serverOK && powerOK
		if ready {
			state = providers.ServerStateReady
		} else {
			state = providers.ServerStateProvisioning
		}
	case "suspended", "stopped":
		state = providers.ServerStateStopped
	}
	created, _ := time.Parse(time.RFC3339, x.DateCreated)
	sshKeys := append([]string(nil), x.SSHKeyIDs...)
	if len(sshKeys) == 0 {
		sshKeys = append(sshKeys, x.SSHKeyIDREST...)
	}
	return providers.Server{ID: x.ID, Name: x.Label, State: state, Ready: ready, RegionID: x.Region, PrimaryIPv4: x.MainIP, CreatedAt: created, Tags: append([]string(nil), x.Tags...), Metadata: map[string]any{"plan": x.Plan, "os": x.OS, "power_status": x.PowerStatus, "server_status": x.ServerStatus, "ssh_key_ids": sshKeys}}
}
func (d *Driver) GetServer(ctx context.Context, id string) (providers.Server, error) {
	x, err := d.client.Instance(ctx, id)
	if err != nil {
		return providers.Server{}, normalizeError("get_server", err)
	}
	return normalizeServer(x), nil
}
func (d *Driver) ListServers(ctx context.Context) ([]providers.Server, error) {
	xs, err := d.client.Instances(ctx)
	if err != nil {
		return nil, normalizeError("list_servers", err)
	}
	out := make([]providers.Server, 0, len(xs))
	for _, x := range xs {
		out = append(out, normalizeServer(x))
	}
	return out, nil
}
func (d *Driver) Inventory(ctx context.Context) (providers.Inventory, error) {
	xs, err := d.ListServers(ctx)
	if err != nil {
		return providers.Inventory{}, err
	}
	return providers.Inventory{Servers: xs, ObservedAt: time.Now().UTC()}, nil
}
func (d *Driver) Observe(ctx context.Context) (providers.Observation, error) {
	a, err := d.Account(ctx)
	if err != nil {
		return providers.Observation{}, err
	}
	cat, err := d.Catalog(ctx)
	if err != nil {
		return providers.Observation{}, err
	}
	inv, err := d.Inventory(ctx)
	if err != nil {
		return providers.Observation{}, err
	}
	now := time.Now().UTC()
	cap := providers.Capacity{LimitKnown: false, ComputeInUse: len(inv.Servers), ObservedAt: now}
	return providers.Observation{Account: a, Capacity: cap, Catalog: cat, Inventory: inv, ObservedAt: now}, nil
}

var _ = fmt.Sprintf
