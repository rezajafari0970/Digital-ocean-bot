package digitalocean

import "context"

type Region struct {
	Slug         string
	Name         string
	Available    bool
	PriceMonthly float64  `json:"price_monthly"`
	PriceHourly  float64  `json:"price_hourly"`
	Transfer     float64  `json:"transfer"`
	Regions      []string `json:"regions"`
}
type Size struct {
	Slug         string   `json:"slug"`
	Memory       int      `json:"memory"`
	VCPUs        int      `json:"vcpus"`
	Disk         int      `json:"disk"`
	Available    bool     `json:"available"`
	PriceMonthly float64  `json:"price_monthly"`
	PriceHourly  float64  `json:"price_hourly"`
	Transfer     float64  `json:"transfer"`
	Regions      []string `json:"regions"`
}
type Image struct {
	ID           int
	Name         string
	Distribution string
	Slug         string
	Public       bool
	Status       string
}
type Droplet struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Region    Region `json:"region"`
	CreatedAt string `json:"created_at"`
	Networks  struct {
		V4 []struct {
			IPAddress string `json:"ip_address"`
			Type      string `json:"type"`
		} `json:"v4"`
	} `json:"networks"`
	PublicIPv4 string `json:"-"`
}
type Project struct {
	ID          string
	Name        string
	Purpose     string
	Environment string
}
type SSHKey struct {
	ID          int
	Name        string
	Fingerprint string
}
type Firewall struct {
	ID     string
	Name   string
	Status string
}

type DiscoveryResult struct {
	Account   AccountInfo
	Limits    Limits
	Regions   []Region
	Sizes     []Size
	Images    []Image
	Droplets  []Droplet
	Projects  []Project
	SSHKeys   []SSHKey
	Firewalls []Firewall
}

func (c *Client) Discover(ctx context.Context) (DiscoveryResult, error) {
	account, err := c.GetAccount(ctx)
	if err != nil {
		return DiscoveryResult{}, err
	}
	result := DiscoveryResult{Account: *account, Limits: Limits{DropletLimit: account.DropletLimit, VolumeLimit: account.VolumeLimit, ReservedIPLimit: account.ReservedIPLimit}}
	if err := c.listAll(ctx, "/regions", "regions", &result.Regions); err != nil {
		return DiscoveryResult{}, err
	}
	if err := c.listAll(ctx, "/sizes", "sizes", &result.Sizes); err != nil {
		return DiscoveryResult{}, err
	}
	if err := c.listAll(ctx, "/images?type=distribution", "images", &result.Images); err != nil {
		return DiscoveryResult{}, err
	}
	if err := c.listAll(ctx, "/droplets", "droplets", &result.Droplets); err != nil {
		return DiscoveryResult{}, err
	}
	if err := c.listAll(ctx, "/projects", "projects", &result.Projects); err != nil {
		return DiscoveryResult{}, err
	}
	if err := c.listAll(ctx, "/account/keys", "ssh_keys", &result.SSHKeys); err != nil {
		return DiscoveryResult{}, err
	}
	if err := c.listAll(ctx, "/firewalls", "firewalls", &result.Firewalls); err != nil {
		return DiscoveryResult{}, err
	}
	return result, nil
}
