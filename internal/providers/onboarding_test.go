package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type discoveryTestFactory struct{}

func (discoveryTestFactory) Name() string                                      { return "fixture" }
func (discoveryTestFactory) Metadata() Metadata                                { return Metadata{} }
func (discoveryTestFactory) Open(context.Context, OpenRequest) (Driver, error) { return nil, nil }
func (discoveryTestFactory) Discover(ctx context.Context, r OpenRequest, _ map[string]string) (Discovery, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://provider.invalid/resource", nil)
	_, e := r.HTTPClient.Do(req)
	return Discovery{Credentials: r.Credentials}, e
}

type discoveryCredential string

func (c discoveryCredential) Get(context.Context) ([]byte, error) { return []byte(c), nil }

type discoveryTransport struct{ calls *int }

func (t discoveryTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*t.calls++
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}
func TestDiscoveryRegistryEnforcesReadOnlyAndSecretBoundary(t *testing.T) {
	r := NewRegistry()
	if e := r.Register(discoveryTestFactory{}); e != nil {
		t.Fatal(e)
	}
	calls := 0
	_, e := r.Discover(context.Background(), "fixture", OpenRequest{Credentials: discoveryCredential("private-key"), HTTPClient: &http.Client{Transport: discoveryTransport{&calls}}}, nil)
	if e == nil || calls != 0 {
		t.Fatal("mutation escaped", e, calls)
	}
	b, e := json.Marshal(Discovery{Complete: true, Credentials: discoveryCredential("private-key")})
	if e != nil || string(b) != `{"complete":true}` {
		t.Fatal("secret boundary", string(b), e)
	}
}
