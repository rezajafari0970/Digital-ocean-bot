package vultr

import (
	"context"
	"net/http"
)

type createInstanceRequest struct {
	Region          string   `json:"region"`
	Plan            string   `json:"plan"`
	OSID            int      `json:"os_id"`
	Label           string   `json:"label,omitempty"`
	Hostname        string   `json:"hostname,omitempty"`
	SSHKeyID        string   `json:"sshkey_id,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	EnableIPv6      bool     `json:"enable_ipv6"`
	ActivationEmail bool     `json:"activation_email"`
}

func (c *Client) CreateInstance(ctx context.Context, x createInstanceRequest) (instance, error) {
	var out struct {
		Instance instance `json:"instance"`
	}
	err := c.do(ctx, http.MethodPost, "/instances", x, &out)
	return out.Instance, err
}
func (c *Client) DeleteInstance(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/instances/"+id, nil, nil)
}
func (c *Client) CreateSSHKey(ctx context.Context, name, key string) (sshKey, error) {
	var out struct {
		SSHKey sshKey `json:"ssh_key"`
	}
	err := c.do(ctx, http.MethodPost, "/ssh-keys", map[string]string{"name": name, "ssh_key": key}, &out)
	return out.SSHKey, err
}
func (c *Client) DeleteSSHKey(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/ssh-keys/"+id, nil, nil)
}
