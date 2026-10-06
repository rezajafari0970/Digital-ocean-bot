package vultr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
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
	userData := ""
	if len(req.SSHKeyRefs) > 0 {
		key, keyErr := d.client.SSHKey(ctx, req.SSHKeyRefs[0])
		if keyErr != nil {
			return providers.CreateServerResult{Outcome: providers.OutcomeRejected}, normalizeError("get_ssh_key", keyErr)
		}
		pub := strings.TrimSpace(key.SSHKey)
		parsed, _, options, rest, parseErr := ssh.ParseAuthorizedKey([]byte(pub))
		if parseErr != nil || len(options) > 0 || len(bytes.TrimSpace(rest)) > 0 {
			return providers.CreateServerResult{Outcome: providers.OutcomeRejected}, &providers.Error{Class: providers.ErrorInvalidRequest, Operation: "create_server", Message: "Vultr SSH public key is empty or unsupported"}
		}
		// Top-level authorized keys target the distribution's default user.
		// Provisioning logs in as root, so explicitly configure that existing user
		// while preserving the default user and keeping password auth disabled.
		pub = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(parsed)))
		quoted, _ := json.Marshal(pub)
		cloud := fmt.Sprintf("#cloud-config\ndisable_root: false\nssh_pwauth: false\nusers:\n  - default\n  - name: root\n    ssh_authorized_keys:\n      - %s\nssh_authorized_keys:\n  - %s\n", quoted, quoted)
		userData = base64.StdEncoding.EncodeToString([]byte(cloud))
	}
	x, e := d.client.CreateInstance(ctx, createInstanceRequest{Region: req.RegionID, Plan: req.PlanID, OSID: osID, Label: req.Name, Hostname: req.Name, SSHKeyIDs: append([]string(nil), req.SSHKeyRefs...), Tags: tags, EnableIPv6: false, ActivationEmail: false, UserData: userData})
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
	for attempt := 0; attempt < 5; attempt++ {
		seen, getErr := d.client.SSHKey(ctx, x.ID)
		if getErr == nil && seen.ID == x.ID && samePublicKey(seen.SSHKey, publicKey) {
			return providers.SSHKey{ID: x.ID, Name: x.Name}, nil
		}
		if attempt == 4 {
			if getErr != nil {
				return providers.SSHKey{}, normalizeError("verify_ssh_key", getErr)
			}
			break
		}
		t := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return providers.SSHKey{}, ctx.Err()
		case <-t.C:
		}
	}
	cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = d.client.DeleteSSHKey(cleanup, x.ID)
	return providers.SSHKey{}, &providers.Error{Class: providers.ErrorUnavailable, Operation: "verify_ssh_key", Message: "Vultr SSH key missing or key material mismatched after create"}
}

func samePublicKey(a, b string) bool {
	ka, _, oa, ra, ea := ssh.ParseAuthorizedKey([]byte(a))
	kb, _, ob, rb, eb := ssh.ParseAuthorizedKey([]byte(b))
	return ea == nil && eb == nil && len(oa) == 0 && len(ob) == 0 && len(bytes.TrimSpace(ra)) == 0 && len(bytes.TrimSpace(rb)) == 0 && bytes.Equal(ka.Marshal(), kb.Marshal())
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
