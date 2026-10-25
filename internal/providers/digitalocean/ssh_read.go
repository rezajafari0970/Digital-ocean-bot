package digitalocean

import (
	"context"
	"errors"
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
		if k.ID <= 0 {
			return nil, errors.New("SSH key inventory has invalid identity")
		}
		out = append(out, providers.SSHKey{ID: strconv.Itoa(k.ID), Name: k.Name, Fingerprint: k.Fingerprint, PublicKey: k.PublicKey})
	}
	return out, nil
}
