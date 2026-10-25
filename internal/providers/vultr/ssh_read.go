package vultr

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
)

func (d *Driver) ListSSHKeys(ctx context.Context) ([]providers.SSHKey, error) {
	pager := newCatalogPager("/ssh-keys")
	out := []providers.SSHKey{}
	for pager.next != "" {
		path, err := pager.path(ctx)
		if err != nil {
			return nil, err
		}
		var page struct {
			Keys *[]sshKey      `json:"ssh_keys"`
			Meta paginationMeta `json:"meta"`
		}
		if err = d.client.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		if page.Keys == nil {
			return nil, errors.New("missing SSH key inventory")
		}
		for _, k := range *page.Keys {
			out = append(out, providers.SSHKey{ID: k.ID, Name: k.Name, PublicKey: k.SSHKey})
		}
		if err = pager.advance(page.Meta.Links.Next); err != nil {
			return nil, err
		}
	}
	return out, nil
}
