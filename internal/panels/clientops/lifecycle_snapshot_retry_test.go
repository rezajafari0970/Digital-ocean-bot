package clientops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLifecycleInventoryRereadsBothViewsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name, field        string
		persistent, cancel bool
		reads              int
		wantErr            error
	}{
		{"enable projection settles", "enable", false, false, 2, nil},
		{"quota edit settles", "quota", false, false, 2, nil},
		{"expiry edit settles", "expiry", false, false, 2, nil},
		{"persistent disagreement closes", "enable", true, false, 3, ErrVerify},
		{"identity conflict never retried", "identity", true, false, 1, ErrClientConflict},
		{"cancelled observation stops", "enable", true, true, 1, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var inboundReads, globalReads, writes atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			original := sanaei.Client{ID: "test-client", Email: "owned@test", Enable: true, ExpiryTime: time.Now().Add(-time.Minute).UnixMilli(), TotalGB: 100}
			changed := original
			switch tc.field {
			case "enable":
				changed.Enable = false
			case "quota":
				changed.TotalGB = 200
			case "expiry":
				changed.ExpiryTime = time.Now().Add(time.Hour).UnixMilli()
			case "identity":
				changed.Email = "other@test"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				obj := func(v any) { json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": v}) }
				switch {
				case strings.HasSuffix(r.URL.Path, "inbounds/list"):
					n := inboundReads.Add(1)
					c := original
					if n > 1 && !tc.persistent {
						c = changed
					}
					obj([]any{map[string]any{"id": 1, "port": 443, "enable": true, "protocol": "vless", "streamSettings": map[string]any{"network": "tcp", "security": "reality"}, "settings": map[string]any{"clients": []sanaei.Client{c}}}})
				case strings.HasSuffix(r.URL.Path, "clients/list"):
					globalReads.Add(1)
					obj([]sanaei.GlobalClient{{UUID: changed.ID, Email: changed.Email, Enable: changed.Enable, TotalGB: changed.TotalGB, ExpiryTime: changed.ExpiryTime, InboundIDs: []int64{1}, Traffic: &sanaei.ClientTraffic{Email: changed.Email}}})
				default:
					writes.Add(1)
					w.WriteHeader(500)
				}
			}))
			defer srv.Close()
			api, e := sanaei.NewAPIClient(srv.URL, sanaei.Credentials{Username: "test", Password: "test"}, nil)
			if e != nil {
				t.Fatal(e)
			}
			session := sanaei.NewPanelSession(api)
			rt := &sanaei.PanelRuntime{Session: session}
			// Cancel on the first global read after the response has been decoded,
			// avoiding a timing dependency on the HTTP server's request completion.
			if tc.cancel {
				rt.Session.Exec.Observe = func(success, transient bool) {
					if success && globalReads.Load() > 0 {
						cancel()
					}
				}
			}
			observed, port, err := LifecycleInventory(ctx, rt, 1)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("wanted %v, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil || port != 443 {
					t.Fatal(port, err)
				}
				got := observed[original.ID].Client
				if got.Enable != changed.Enable || got.TotalGB != changed.TotalGB || got.ExpiryTime != changed.ExpiryTime {
					t.Fatal("did not return the converged fresh policy")
				}
			}
			if int(inboundReads.Load()) != tc.reads || int(globalReads.Load()) != tc.reads {
				t.Fatalf("reads inbound=%d global=%d", inboundReads.Load(), globalReads.Load())
			}
			if writes.Load() != 0 {
				t.Fatal("inventory mutated remote state")
			}
		})
	}
}
