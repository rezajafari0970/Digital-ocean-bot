package providers

import (
	"context"
	"errors"
	"net/http"
)

// Discovery is read-only and returns only public choices. Credentials never
// cross the response boundary; the browser retains the original secret input.
type DiscoveryOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type DiscoveryField struct {
	Key     string            `json:"key"`
	Label   string            `json:"label"`
	Value   string            `json:"value"`
	Options []DiscoveryOption `json:"options"`
}
type Discovery struct {
	NetworkApproval bool             `json:"network_approval,omitempty"`
	Complete        bool             `json:"complete"`
	Fields          []DiscoveryField `json:"fields,omitempty"`
	Resolved        map[string]any   `json:"resolved,omitempty"`
	Message         string           `json:"message,omitempty"`
	Credentials     CredentialSource `json:"-"`
}
type OnboardingFactory interface {
	Discover(context.Context, OpenRequest, map[string]string) (Discovery, error)
}

type readOnlyDiscoveryTransport struct{ base http.RoundTripper }

func (t readOnlyDiscoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nil, errors.New("discovery is read-only")
	}
	return t.base.RoundTrip(r)
}
func (r *Registry) Discover(ctx context.Context, name string, req OpenRequest, selection map[string]string) (Discovery, error) {
	r.mu.RLock()
	f := r.factories[normalizeName(name)]
	r.mu.RUnlock()
	if f == nil {
		return Discovery{}, errors.New("provider not registered")
	}
	d, ok := f.(OnboardingFactory)
	if !ok {
		return Discovery{Complete: true, Credentials: req.Credentials}, nil
	}
	if req.HTTPClient == nil || req.Credentials == nil {
		return Discovery{}, errors.New("invalid discovery request")
	}
	c := *req.HTTPClient
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.Transport = readOnlyDiscoveryTransport{base}
	req.HTTPClient = &c
	return d.Discover(ctx, req, selection)
}
