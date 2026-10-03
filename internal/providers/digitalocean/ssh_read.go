package digitalocean

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strconv"
)

func (d *Driver) ListSSHKeys(ctx context.Context) ([]providers.SSHKey, error) {
	var keys []SSHKey
	if err := d.client.listAll(ctx, "/account/keys", "ssh_keys", &keys); err != nil {
		return nil, normalizeError("list_ssh_keys", err)
	}
	out := make([]providers.SSHKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, providers.SSHKey{ID: strconv.Itoa(k.ID), Name: k.Name, Fingerprint: k.Fingerprint})
	}
	return out, nil
}
