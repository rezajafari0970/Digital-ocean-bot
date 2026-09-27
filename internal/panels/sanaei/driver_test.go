package sanaei

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDriverDiscoveryClassifiesRuntimeCapabilities(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/panel/api/inbounds/list/slim":
			w.WriteHeader(200)
		case "/panel/api/inbounds/list":
			w.WriteHeader(200)
		case "/panel/api/server/status":
			w.WriteHeader(200)
		case "/panel/api/server/scanRealityTargets":
			w.WriteHeader(400)
		default:
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	d := Driver{HTTP: s.Client()}
	got, err := d.Discover(context.Background(), panels.Instance{BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Capabilities.InventorySlim || !got.Capabilities.InventoryFull || !got.Capabilities.ServerStatus || !got.Capabilities.RealityScan {
		t.Fatalf("unexpected capabilities: %+v", got.Capabilities)
	}
}

func TestDiscoverWithExecutorIsReadOnly(t *testing.T) {
	f := fakeExecutor{resp: SessionResponse{StatusCode: 200}}
	d, err := DiscoverWithExecutor(context.Background(), f, "3.8.5")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Capabilities.InventorySlim || !d.Capabilities.InventoryFull || !d.Capabilities.ServerStatus {
		t.Fatalf("unexpected: %+v", d.Capabilities)
	}
}
