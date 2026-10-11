package residentialsync

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Reproduce the deployed panel's shared gRPC connection close on overlapping
// routeTest calls, including its HTTP 200 failure envelope.
func TestNativeHTTPProofRespectsPanelGRPCSerialContract(t *testing.T) {
	p := routePolicy{StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Direct: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	clients, tags := fixtureClients(t, p)
	desired, err := buildSettings(baseSettings(), clients, tags, p)
	if err != nil {
		t.Fatal(err)
	}
	core := &fakeCore{saved: desired, running: desired}
	var active, reads, overlap atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(path, "routeTest") {
			reads.Add(1)
			n := active.Add(1)
			defer active.Add(-1)
			if n > 1 {
				overlap.Add(1)
				fmt.Fprint(w, `{"success":false,"msg":"Something went wrong (rpc error: code = Canceled desc = grpc: the client connection is closing)"}`)
				return
			}
			time.Sleep(3 * time.Millisecond)
		}
		body, _ := io.ReadAll(r.Body)
		resp, e := core.Do(r.Context(), sanaei.SessionRequest{Method: r.Method, Path: path, Body: body})
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(resp.Body)
	}))
	defer srv.Close()
	session := &sanaei.ResilientSession{Client: &sanaei.APIClient{BaseURL: srv.URL, HTTP: srv.Client()}}
	if err = applyAndVerify(context.Background(), session, desired, desired, "", clients, tags, p); err != nil {
		t.Fatal(err)
	}
	if overlap.Load() != 0 || reads.Load() != int32(len(tags)*3*11+1) || core.saves != 0 || core.restarts != 0 {
		t.Fatal("native contract violated", overlap.Load(), reads.Load(), core.saves, core.restarts)
	}
}
