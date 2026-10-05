package upcloud

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"testing"
)

func TestCleanupOwnershipAndDetachedDisk(t *testing.T) {
	for _, mode := range []string{"foreign-attached", "owned-detached", "reattached-elsewhere"} {
		t.Run(mode, func(t *testing.T) {
			labels := []label{{"dob-owner", "account-A"}, {"dob-identity", "identity"}}
			disk := storageData{ID: storageID, Type: "normal", State: "online", Labels: labels}
			s := serverData{ID: serverID, State: "stopped"}
			s.Labels.Items = labels
			if mode == "foreign-attached" {
				raw, _ := json.Marshal(map[string]any{"storage_device": []any{map[string]any{"storage": storageID, "type": "disk"}}})
				json.Unmarshal(raw, &s.Disks)
				disk.Labels = []label{{"dob-owner", "someone-else"}}
			}
			if mode == "reattached-elsewhere" {
				disk.Servers.Items = []string{"another-server"}
			}
			serverGone, diskGone := false, false
			mutations := 0
			j := &journal{}
			d := fixture(t, func(r *http.Request) (int, any, error) {
				if r.Method != "GET" {
					mutations++
				}
				switch r.URL.Path {
				case "/1.3/server/" + serverID:
					if r.Method == "DELETE" {
						serverGone = true
						return 204, nil, nil
					}
					if serverGone {
						return 404, map[string]any{}, nil
					}
					return 200, map[string]any{"server": s}, nil
				case "/1.3/storage/private":
					return 200, map[string]any{"storages": map[string]any{"storage": []storageData{disk}}}, nil
				case "/1.3/storage/" + storageID:
					if r.Method == "DELETE" {
						diskGone = true
						return 204, nil, nil
					}
					if diskGone {
						return 404, map[string]any{}, nil
					}
					return 200, map[string]any{"storage": disk}, nil
				}
				t.Fatalf("unexpected %s", r.URL.Path)
				return 500, nil, nil
			}, j)
			e := d.DeleteServer(context.Background(), serverID)
			switch mode {
			case "foreign-attached":
				if e == nil || mutations != 0 {
					t.Fatal("foreign disk safety", e, mutations)
				}
			case "owned-detached":
				if e != nil || !diskGone || !serverGone {
					t.Fatal("detached disk leaked", e, diskGone)
				}
			case "reattached-elsewhere":
				if !providers.IsRetryable(e) || diskGone {
					t.Fatal("attached disk was not preserved", e)
				}
				got, e := d.GetServer(context.Background(), serverID)
				if e != nil || got.State != providers.ServerStateDeleting {
					t.Fatal("cleanup completed prematurely", got, e)
				}
			}
		})
	}
}
