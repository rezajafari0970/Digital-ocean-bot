package vultr

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) Account(ctx context.Context) (accountResponse, error) {
	var x accountResponse
	err := c.do(ctx, http.MethodGet, "/account", nil, &x)
	return x, err
}
func (c *Client) Regions(ctx context.Context) ([]region, error) {
	var x regionsResponse
	err := c.do(ctx, http.MethodGet, pagePath("/regions", 500), nil, &x)
	return x.Regions, err
}
func (c *Client) Plans(ctx context.Context) ([]plan, error) {
	var x plansResponse
	err := c.do(ctx, http.MethodGet, pagePath("/plans", 500), nil, &x)
	return x.Plans, err
}
func (c *Client) OS(ctx context.Context) ([]osItem, error) {
	var x osResponse
	err := c.do(ctx, http.MethodGet, pagePath("/os", 500), nil, &x)
	return x.OS, err
}
func (c *Client) Instances(ctx context.Context) ([]instance, error) {
	var x instancesResponse
	err := c.do(ctx, http.MethodGet, pagePath("/instances", 500), nil, &x)
	return x.Instances, err
}
func (c *Client) Instance(ctx context.Context, id string) (instance, error) {
	var x struct {
		Instance instance `json:"instance"`
	}
	err := c.do(ctx, http.MethodGet, "/instances/"+id, nil, &x)
	return x.Instance, err
}
func (c *Client) RegionAvailability(ctx context.Context, id string) ([]string, error) {
	var x struct {
		AvailablePlans []string `json:"available_plans"`
	}
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/regions/%s/availability", id), nil, &x)
	return x.AvailablePlans, err
}
