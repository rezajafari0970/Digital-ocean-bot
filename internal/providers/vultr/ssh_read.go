package vultr

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"net/url"
)

func (d *Driver) ListSSHKeys(ctx context.Context) ([]providers.SSHKey, error) {
	path := "/ssh-keys?per_page=500"
	seen := map[string]bool{}
	out := []providers.SSHKey{}
	for len(seen) < 1000 {
		if seen[path] {
			return nil, errors.New("SSH key pagination cycle")
		}
		seen[path] = true
		var page struct {
			Keys *[]sshKey      `json:"ssh_keys"`
			Meta paginationMeta `json:"meta"`
		}
		if err := d.client.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		if page.Keys == nil {
			return nil, errors.New("missing SSH key inventory")
		}
		for _, k := range *page.Keys {
			out = append(out, providers.SSHKey{ID: k.ID, Name: k.Name})
		}
		if page.Meta.Links.Next == "" {
			return out, nil
		}
		u, err := url.Parse(page.Meta.Links.Next)
		if err != nil {
			return nil, err
		}
		if u.RawQuery != "" {
			path = "/ssh-keys?" + u.RawQuery
		} else {
			path = "/ssh-keys?" + url.Values{"per_page": {"500"}, "cursor": {page.Meta.Links.Next}}.Encode()
		}
	}
	return nil, errors.New("SSH key pagination limit")
}
