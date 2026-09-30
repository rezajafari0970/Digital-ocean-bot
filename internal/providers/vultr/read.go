package vultr

import (
	"context"
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
	path := pagePath("/regions", 500)
	var out []region
	for {
		var x regionsResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		out = append(out, x.Regions...)
		if strings.TrimSpace(x.Meta.Links.Next) == "" {
			return out, nil
		}
		u, err := url.Parse(x.Meta.Links.Next)
		if err != nil {
			return nil, err
		}
		path = u.RequestURI()
	}
}
func (c *Client) Plans(ctx context.Context) ([]plan, error) {
	path := pagePath("/plans", 500)
	var out []plan
	for {
		var x plansResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		out = append(out, x.Plans...)
		if strings.TrimSpace(x.Meta.Links.Next) == "" {
			return out, nil
		}
		u, err := url.Parse(x.Meta.Links.Next)
		if err != nil {
			return nil, err
		}
		path = u.RequestURI()
	}
}
func (c *Client) OS(ctx context.Context) ([]osItem, error) {
	path := pagePath("/os", 500)
	var out []osItem
	for {
		var x osResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		out = append(out, x.OS...)
		if strings.TrimSpace(x.Meta.Links.Next) == "" {
			return out, nil
		}
		u, err := url.Parse(x.Meta.Links.Next)
		if err != nil {
			return nil, err
		}
		path = u.RequestURI()
	}
}
func (c *Client) Instances(ctx context.Context) ([]instance, error) {
	path := pagePath("/instances", 500)
	var out []instance
	for {
		var x instancesResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &x); err != nil {
			return nil, err
		}
		out = append(out, x.Instances...)
		if strings.TrimSpace(x.Meta.Links.Next) == "" {
			return out, nil
		}
		u, err := url.Parse(x.Meta.Links.Next)
		if err != nil {
			return nil, err
		}
		path = u.RequestURI()
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
