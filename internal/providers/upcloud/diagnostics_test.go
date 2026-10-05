package upcloud

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResponseSchemaFailuresAreNotTransportFailures(t *testing.T) {
	for _, body := range []string{"", "<html>ucat_secret</html>", "{\"account\":{\"username\":\"example\",\"resource_limits\":{\"cores\":null}}}"} {
		d := fixture(t, func(*http.Request) (int, any, error) { return 200, nil, nil }, nil)
		d.client.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
		})
		_, err := d.Account(context.Background())
		if providers.Class(err) != providers.ErrorUnavailable || Diagnostic(err) == "REQUEST_FAILED" {
			t.Fatalf("class=%s diagnostic=%s", providers.Class(err), Diagnostic(err))
		}
		if strings.Contains(err.Error(), "ucat_secret") {
			t.Fatal("secret leaked")
		}
	}
}
func TestMalformedListDiagnosticAndCatalogEmpty(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		switch r.URL.Path {
		case "/1.3/zone":
			return 200, map[string]any{"zones": map[string]any{"zone": []any{}}}, nil
		case "/1.3/plan":
			return 200, planJSON(), nil
		case "/1.3/storage/template":
			return 200, map[string]any{"storages": map[string]any{"storage": []any{}}}, nil
		}
		return 200, map[string]any{"servers": map[string]any{"server": nil}}, nil
	}, nil)
	if _, e := d.Catalog(context.Background()); Diagnostic(e) != "NO_CATALOG_z0_p2_i0" {
		t.Fatal(Diagnostic(e))
	}
	if _, e := d.ListServers(context.Background()); Diagnostic(e) != "MISSING_LIST" {
		t.Fatal(Diagnostic(e))
	}
}
func TestUnsupportedNumbersRemainRejected(t *testing.T) {
	for _, s := range []string{"null", "-1", "1.5", "\"not-a-number\""} {
		var n number
		if json.Unmarshal([]byte(s), &n) == nil {
			t.Fatal("unknown quota accepted", s)
		}
	}
}
