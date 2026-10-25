package vultr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) Account(ctx context.Context) (accountResponse, error) {
	var x accountResponse
	err := c.do(ctx, http.MethodGet, "/account", nil, &x)
	return x, err
}
func (c *Client) Regions(ctx context.Context) ([]region, error) {
	pager := newCatalogPager("/regions")
	var out []region
	for pager.next != "" {
		path, err := pager.path(ctx)
		if err != nil {
			return nil, err
		}
		var x regionsResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		if x.Regions == nil {
			return nil, errors.New("missing regions inventory")
		}
		out = append(out, x.Regions...)
		if err := pager.advance(x.Meta.Links.Next); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (c *Client) Plans(ctx context.Context) ([]plan, error) {
	pager := newCatalogPager("/plans")
	var out []plan
	for pager.next != "" {
		path, err := pager.path(ctx)
		if err != nil {
			return nil, err
		}
		var x plansResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		if x.Plans == nil {
			return nil, errors.New("missing plans inventory")
		}
		out = append(out, x.Plans...)
		if err := pager.advance(x.Meta.Links.Next); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (c *Client) OS(ctx context.Context) ([]osItem, error) {
	pager := newCatalogPager("/os")
	var out []osItem
	for pager.next != "" {
		path, err := pager.path(ctx)
		if err != nil {
			return nil, err
		}
		var x osResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		if x.OS == nil {
			return nil, errors.New("missing os inventory")
		}
		out = append(out, x.OS...)
		if err := pager.advance(x.Meta.Links.Next); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (c *Client) Instances(ctx context.Context) ([]instance, error) {
	path := pagePath("/instances", 500)
	var out []instance
	seen := map[string]bool{}
	for {
		if seen[path] || len(seen) >= 1000 {
			return nil, errors.New("instance pagination cycle or limit")
		}
		seen[path] = true
		var x instancesResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		if x.Instances == nil {
			return nil, errors.New("missing instance inventory")
		}
		out = append(out, x.Instances...)
		if strings.TrimSpace(x.Meta.Links.Next) == "" {
			return out, nil
		}
		u, err := url.Parse(x.Meta.Links.Next)
		if err != nil {
			return nil, err
		}
		if u.RawQuery != "" {
			path = "/instances?" + u.RawQuery
		} else {
			path = "/instances?" + url.Values{"per_page": {"500"}, "cursor": {x.Meta.Links.Next}}.Encode()
		}
	}
}
func (c *Client) Instance(ctx context.Context, id string) (instance, error) {
	var x struct {
		Instance instance `json:"instance"`
	}
	err := c.do(ctx, http.MethodGet, "/instances/"+id, nil, &x)
	return x.Instance, err
}

func (c *Client) SSHKey(ctx context.Context, id string) (sshKey, error) {
	var x struct {
		SSHKey sshKey `json:"ssh_key"`
	}
	err := c.do(ctx, http.MethodGet, "/ssh-keys/"+id, nil, &x)
	return x.SSHKey, err
}

func (c *Client) RegionAvailability(ctx context.Context, id string) ([]string, error) {
	var x struct {
		AvailablePlans []string `json:"available_plans"`
	}
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/regions/%s/availability", id), nil, &x)
	return x.AvailablePlans, err
}
