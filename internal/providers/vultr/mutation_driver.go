package vultr

import (
	"context"
	"strconv"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func (d *Driver) CreateServer(ctx context.Context, req providers.CreateServerRequest) (providers.CreateServerResult, error) {
	osID, err := strconv.Atoi(req.ImageID)
	if err != nil {
		return providers.CreateServerResult{Outcome: providers.OutcomeRejected}, &providers.Error{Class: providers.ErrorInvalidRequest, Operation: "create_server", Message: "invalid Vultr OS id", Cause: err}
	}
	tags := append([]string(nil), req.Tags...)
	if req.Identity != "" && !containsString(tags, req.Identity) {
		tags = append(tags, req.Identity)
	}
	x, e := d.client.CreateInstance(ctx, createInstanceRequest{Region: req.RegionID, Plan: req.PlanID, OSID: osID, Label: req.Name, Hostname: req.Name, SSHKeyIDs: append([]string(nil), req.SSHKeyRefs...), Tags: tags, EnableIPv6: false, ActivationEmail: false})
	if e != nil {
		pe := normalizeError("create_server", e)
		out := providers.OutcomeRejected
		if providers.IsRetryable(pe) {
			out = providers.OutcomeAmbiguous
		}
		return providers.CreateServerResult{Outcome: out}, pe
	}
	if x.ID == "" {
		return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, &providers.Error{Class: providers.ErrorAmbiguousOutcome, Operation: "create_server", Message: "Vultr create returned no instance id"}
	}
	return providers.CreateServerResult{ServerID: x.ID, Outcome: providers.OutcomeAccepted}, nil
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	err := d.client.DeleteInstance(ctx, id)
	if err == nil {
		return nil
	}
	pe := normalizeError("delete_server", err)
	if providers.IsClass(pe, providers.ErrorNotFound) {
		return nil
	}
	return pe
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	xs, err := d.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]providers.Server, 0, 1)
	for _, x := range xs {
		if containsString(x.Tags, identity) {
			out = append(out, x)
		}
	}
	return out, nil
}
func (d *Driver) CreateSSHKey(ctx context.Context, name, publicKey string) (providers.SSHKey, error) {
	x, err := d.client.CreateSSHKey(ctx, name, publicKey)
	if err != nil {
		return providers.SSHKey{}, normalizeError("create_ssh_key", err)
	}
	return providers.SSHKey{ID: x.ID, Name: x.Name}, nil
}
func (d *Driver) DeleteSSHKey(ctx context.Context, id string) error {
	err := d.client.DeleteSSHKey(ctx, id)
	if err == nil {
		return nil
	}
	pe := normalizeError("delete_ssh_key", err)
	if providers.IsClass(pe, providers.ErrorNotFound) {
		return nil
	}
	return pe
}
func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
