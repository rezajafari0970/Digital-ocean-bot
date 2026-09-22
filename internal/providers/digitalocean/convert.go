package digitalocean

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resources"
	"strconv"
)

func RegistryResources(accountID string, d DiscoveryResult) []resources.Resource {
	out := make([]resources.Resource, 0, len(d.Droplets)+len(d.Firewalls))
	for _, x := range d.Droplets {
		out = append(out, resources.Resource{AccountID: accountID, Provider: "digitalocean", ProviderResourceID: strconv.Itoa(x.ID), Type: "droplet", State: x.Status, Metadata: map[string]any{"name": x.Name, "region": x.Region.Slug, "created_at": x.CreatedAt}})
	}
	for _, x := range d.Firewalls {
		out = append(out, resources.Resource{AccountID: accountID, Provider: "digitalocean", ProviderResourceID: x.ID, Type: "firewall", State: x.Status, Metadata: map[string]any{"name": x.Name}})
	}
	return out
}
