package kamatera

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const capabilitiesFixture = `{"diskSizeGB":[20,30,60,100,150,200],"monthlyTrafficPackage":{"t5000":"included"},"defaultMonthlyTrafficPackage":"t5000","cpuTypes":[{"id":"B","cpuCores":[1,2,4,8,16,32],"ramMB":[1024,2048,4096,8192,16384,32768,65536]}]}`

func TestCapabilitySuggestionsAreBoundedAndRegional(t *testing.T) {
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("capabilities") != "1" || r.URL.Query().Get("datacenter") != "EU" {
			t.Fatal("wrong regional request")
		}
		return reply(capabilitiesFixture), nil
	}, nil)
	xs, e := d.suggestedSizes(context.Background(), "EU")
	if e != nil || len(xs) != 8 {
		t.Fatal(len(xs), e)
	}
	if xs[0].planID() != "1B-1024-20" || xs[0].Traffic != "t5000" {
		t.Fatal(xs[0])
	}
	// Changes to one region's native dimensions remove its unsupported suggestions.
	d.client.http.Transport = transportFixture(func(*http.Request) (*http.Response, error) {
		return reply(strings.Replace(capabilitiesFixture, `"cpuCores":[1,2,4,8,16,32]`, `"cpuCores":[1]`, 1)), nil
	})
	xs, e = d.suggestedSizes(context.Background(), "EU")
	if e != nil || len(xs) != 2 {
		t.Fatal(len(xs), e)
	}
	for _, bad := range []string{`{}`, strings.Replace(capabilitiesFixture, `"defaultMonthlyTrafficPackage":"t5000"`, `"defaultMonthlyTrafficPackage":"missing"`, 1)} {
		d.client.http.Transport = transportFixture(func(*http.Request) (*http.Response, error) { return reply(bad), nil })
		if _, e = d.suggestedSizes(context.Background(), "EU"); e == nil {
			t.Fatal("accepted unsupported capabilities")
		}
	}
}
func TestSuggestedShapeCreateUsesNativeFieldsAndRechecksCapabilities(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "removed", true: "supported"}[available], func(t *testing.T) {
			j := &memoryJournal{}
			posts := 0
			d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
				switch {
				case r.URL.Query().Get("sizes") == "1":
					return reply(`[]`), nil
				case r.URL.Query().Get("images") == "1":
					return reply(imageJSON), nil
				case r.URL.Query().Get("capabilities") == "1":
					b := capabilitiesFixture
					if !available {
						b = strings.Replace(b, `"cpuCores":[1,2,4,8,16,32]`, `"cpuCores":[2]`, 1)
					}
					return reply(b), nil
				case r.URL.Path == "/service/server/info":
					return reply(`[]`), nil
				case r.URL.Path == "/service/server" && r.Method == "POST":
					posts++
					var b map[string]any
					json.NewDecoder(r.Body).Decode(&b)
					if b["cpu"] != "1B" || b["ram"] != float64(1024) || b["disk"] != "size=20" || b["monthlypackage"] != "t5000" {
						t.Fatal("wrong explicit shape", b)
					}
					return reply(`[12345]`), nil
				default:
					return reply(`[]`), nil
				}
			}, j)
			result, e := d.CreateServer(context.Background(), providers.CreateServerRequest{Identity: identity, RegionID: "EU", PlanID: "1B-1024-20", ImageID: "EU:ubuntu-24.04-64bit", SSHAuthorizedKeys: []string{fixtureKey(t)}})
			if available {
				if posts != 1 || !providers.IsClass(e, providers.ErrorAmbiguousOutcome) || result.OperationID != "12345" {
					t.Fatal(posts, e, result)
				}
			} else if posts != 0 || !result.ComputeNotDispatched || !providers.IsClass(e, providers.ErrorInvalidRequest) {
				t.Fatal(posts, e, result)
			}
		})
	}
}
func TestKamateraRetryOnlyTransientGET(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		status, want int
		transport    bool
	}{
		{"transport", "GET", 0, 2, true}, {"server", "GET", 503, 2, false}, {"auth", "GET", 401, 1, false}, {"permission", "GET", 403, 1, false}, {"malformed", "GET", 200, 1, false}, {"create", "POST", 503, 1, false}, {"post-transport", "POST", 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			d := fixtureDriver(t, func(*http.Request) (*http.Response, error) {
				calls++
				if tc.transport {
					return nil, errors.New("private-token")
				}
				r := reply("invalid")
				r.StatusCode = tc.status
				return r, nil
			}, nil)
			var out []any
			e := d.client.do(context.Background(), tc.method, "/service/servers", nil, &out)
			if e == nil || calls != tc.want || strings.Contains(e.Error(), "private-token") {
				t.Fatal(calls, e)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	d := fixtureDriver(t, func(*http.Request) (*http.Response, error) { calls++; cancel(); return nil, io.ErrUnexpectedEOF }, nil)
	at := time.Now()
	_ = d.client.do(ctx, "GET", "/service/servers", nil, nil)
	if calls != 1 || time.Since(at) > time.Second {
		t.Fatal("retry ignored cancellation")
	}
}
func TestSuggestionsRespectImageMinimums(t *testing.T) {
	var ims []imageData
	json.Unmarshal([]byte(imageJSON), &ims)
	if !supportedSize(sizeData{CPUType: "B", Cores: 1, RAM: 1024, Disk: 20}, ims) {
		t.Fatal("valid small plan")
	}
	ims[0].RAM["B"] = 2048
	if supportedSize(sizeData{CPUType: "B", Cores: 1, RAM: 1024, Disk: 20}, ims) {
		t.Fatal("image memory ignored")
	}
}
