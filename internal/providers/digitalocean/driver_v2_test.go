package digitalocean

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestNormalizeCatalogToCanonicalModels(t *testing.T) {
	raw := DiscoveryResult{Regions: []Region{{Slug: "fra1", Name: "Frankfurt", Available: true}}, Sizes: []Size{{Slug: "s-1", Memory: 1024, VCPUs: 1, Disk: 25, Available: true, PriceHourly: .01, PriceMonthly: 6, Regions: []string{"fra1"}}}, Images: []Image{{Name: "24.04 (LTS) x64", Distribution: "Ubuntu", Slug: "ubuntu-24-04-x64", Status: "available"}}}
	got := normalizeCatalog(raw)
	if len(got.Regions) != 1 || got.Regions[0].ID != "fra1" {
		t.Fatalf("regions=%+v", got.Regions)
	}
	if len(got.Plans) != 1 || got.Plans[0].ID != "s-1" || got.Plans[0].MemoryMB != 1024 || got.Plans[0].AvailableRegions[0] != "fra1" {
		t.Fatalf("plans=%+v", got.Plans)
	}
	if len(got.Images) != 1 || got.Images[0].Family != "ubuntu" || got.Images[0].Version != "24.04" || got.Images[0].Architecture != "x86_64" {
		t.Fatalf("images=%+v", got.Images)
	}
}
func TestNormalizeServerUsesStringIDAndReadiness(t *testing.T) {
	d := Droplet{ID: 42, Name: "srv", Status: "active", Region: Region{Slug: "fra1"}, Tags: []string{"dob-deployment-x"}, PublicIPv4: "203.0.113.7", CreatedAt: "2026-09-29T00:00:00Z"}
	got := normalizeServer(d)
	if got.ID != "42" || !got.Ready || got.State != providers.ServerStateReady || got.PrimaryIPv4 != "203.0.113.7" || got.RegionID != "fra1" {
		t.Fatalf("server=%+v", got)
	}
}
func TestReadyRequiresPublicIPv4(t *testing.T) {
	d := Droplet{ID: 42, Name: "srv", Status: "active", Region: Region{Slug: "fra1"}}
	if normalizeServer(d).Ready {
		t.Fatal("active without public ipv4 must not be ready")
	}
}
func TestNormalizeProviderErrors(t *testing.T) {
	cases := []struct {
		err  error
		want providers.ErrorClass
	}{
		{HTTPError{Status: 401, Message: "bad token"}, providers.ErrorAuthentication},
		{HTTPError{Status: 403, Message: "account locked"}, providers.ErrorAccountLocked},
		{HTTPError{Status: 403, Message: "forbidden"}, providers.ErrorPermissionDenied},
		{HTTPError{Status: 404, Message: "missing"}, providers.ErrorNotFound},
		{HTTPError{Status: 429, Message: "slow down"}, providers.ErrorRateLimited},
		{HTTPError{Status: 422, Code: "region_unavailable", Message: "region unavailable"}, providers.ErrorRegionCapacity},
		{HTTPError{Status: 422, Message: "invalid request"}, providers.ErrorInvalidRequest},
		{HTTPError{Status: 503, Message: "down"}, providers.ErrorUnavailable},
		{errors.New("plain"), providers.ErrorUnknown},
	}
	for _, tc := range cases {
		if got := providers.Class(normalizeError("test", tc.err)); got != tc.want {
			t.Fatalf("err=%v got=%s want=%s", tc.err, got, tc.want)
		}
	}
}
func TestDriverFindServerByIdentity(t *testing.T) {
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		if r.URL.Path != "/v2/droplets" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		return 200, `{"droplets":[{"id":1,"name":"a","status":"active","region":{"slug":"fra1"},"tags":["other"]},{"id":42,"name":"b","status":"active","region":{"slug":"fra1"},"tags":["dob-deployment-x"]}]}`
	})
	d, _ := NewDriver(c)
	got, err := d.FindServerByIdentity(context.Background(), "dob-deployment-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "42" {
		t.Fatalf("servers=%+v", got)
	}
}
