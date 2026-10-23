package ovh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
)

type testCredential string

func (c testCredential) Get(context.Context) ([]byte, error) { return []byte(c), nil }

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const fixtureCredential = `{"endpoint":"ovh-eu","application_key":"fixture-app","application_secret":"fixture-secret","consumer_key":"fixture-consumer","project_id":"0123456789abcdef0123456789abcdef"}`
const serverID = "11111111-1111-1111-1111-111111111111"
const flavorID = "22222222-2222-2222-2222-222222222222"
const imageID = "33333333-3333-3333-3333-333333333333"

func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func openTest(t *testing.T, fn transport) *Driver {
	t.Helper()
	x, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "account-a", HTTPClient: &http.Client{Transport: fn}, Credentials: testCredential(fixtureCredential)})
	if e != nil {
		t.Fatal(e)
	}
	return x.(*Driver)
}
func key(t *testing.T) string {
	p, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	k, _ := ssh.NewPublicKey(p)
	return string(ssh.MarshalAuthorizedKey(k))
}
func readFixture(path string) string {
	switch {
	case strings.HasSuffix(path, "/auth/time"):
		return strconv.FormatInt(time.Now().Unix(), 10)
	case strings.HasSuffix(path, "/flavor"):
		return `[{"id":"` + flavorID + `","name":"b3-8","region":"GRA11","osType":"linux","type":"ovh.cpu","vcpus":2,"ram":8,"disk":50,"available":true}]`
	case strings.HasSuffix(path, "/image"):
		return `[{"id":"` + imageID + `","name":"Ubuntu 24.04","type":"linux","user":"ubuntu","region":"GRA11","status":"active","visibility":"public","creationDate":"2026-10-01T00:00:00Z","minRam":1024,"minDisk":10}]`
	case strings.HasSuffix(path, "/quota"):
		return `[{"region":"GRA11","instance":{"maxCores":8,"usedCores":0,"maxInstances":4,"usedInstances":0,"maxRam":32768,"usedRAM":0}}]`
	case strings.HasSuffix(path, "/region"):
		return `["GRA11"]`
	case strings.HasSuffix(path, "/region/GRA11"):
		return `{"name":"GRA11","status":"UP","type":"region","countryCode":"FR","datacenterLocation":"Gravelines"}`
	}
	return `{}`
}
func TestOVHCreateContractAndAmbiguity(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(strconv.FormatBool(ambiguous), func(t *testing.T) {
			posts := 0
			clock := 0
			var d *Driver
			d = openTest(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "eu.api.ovh.com" {
					t.Fatal("endpoint escaped")
				}
				if strings.HasSuffix(r.URL.Path, "/auth/time") {
					clock++
					return response(readFixture(r.URL.Path)), nil
				}
				b, _ := io.ReadAll(r.Body)
				want := signature("fixture-secret", "fixture-consumer", r.Method, r.URL.String(), string(b), r.Header.Get("X-Ovh-Timestamp"))
				if r.Header.Get("X-Ovh-Signature") != want {
					t.Fatal("wrong signature")
				}
				if r.Method == "POST" {
					posts++
					var q map[string]any
					if json.Unmarshal(b, &q) != nil {
						t.Fatal("invalid payload")
					}
					if q["monthlyBilling"] != false || q["name"] != d.name("identity-fixed") || q["flavorId"] != flavorID || q["imageId"] != imageID || q["region"] != "GRA11" || q["sshKeyId"] != nil || q["volumeId"] != nil || !strings.HasPrefix(q["userData"].(string), "#cloud-config") {
						t.Fatal(q)
					}
					if ambiguous {
						return nil, errors.New("connection lost fixture-secret")
					}
					return response(`{"id":"` + serverID + `","name":"` + d.name("identity-fixed") + `","region":"GRA11","monthlyBilling":null}`), nil
				}
				return response(readFixture(r.URL.Path)), nil
			})
			got, e := d.CreateServer(context.Background(), providers.CreateServerRequest{Identity: "identity-fixed", RegionID: "GRA11", PlanID: flavorID, ImageID: imageID, SSHAuthorizedKeys: []string{key(t)}})
			if posts != 1 || clock != 1 {
				t.Fatal("replayed or redundant calls", posts, clock)
			}
			if ambiguous {
				if got.Outcome != providers.OutcomeAmbiguous || !providers.IsClass(e, providers.ErrorAmbiguousOutcome) || strings.Contains(e.Error(), "fixture-secret") {
					t.Fatal(got, e)
				}
			} else if e != nil || got.ServerID != serverID {
				t.Fatal(got, e)
			}
		})
	}
}
func TestOVHRegionalCatalogAndQuota(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) { return response(readFixture(r.URL.Path)), nil })
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Images) != 1 || len(c.Plans) != 1 || c.Plans[0].IDsByRegion["GRA11"] != flavorID || c.Images[0].IDsByRegion["GRA11"] != imageID {
		t.Fatal(c, e)
	}
	if e = d.checkQuota(context.Background(), "GRA11", flavorData{CPU: 16, RAM: 8}); !providers.IsClass(e, providers.ErrorCapacity) {
		t.Fatal(e)
	}
}
func TestOVHInventoryAndOwnership(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[{"id":"` + serverID + `","name":"foreign","region":"GRA11","status":"ACTIVE"},{"id":"` + serverID + `","name":"foreign","region":"GRA11","status":"ACTIVE"}]`} {
		d := openTest(t, func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/auth/time") {
				return response(readFixture(r.URL.Path)), nil
			}
			return response(body), nil
		})
		if xs, e := d.ListServers(context.Background()); e == nil || xs != nil {
			t.Fatal(xs, e)
		}
	}
	mutations := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			mutations++
		}
		if strings.HasSuffix(r.URL.Path, "/auth/time") {
			return response(readFixture(r.URL.Path)), nil
		}
		return response(`{"id":"` + serverID + `","name":"foreign","region":"GRA11","status":"ACTIVE"}`), nil
	})
	if e := d.DeleteServer(context.Background(), serverID); !providers.IsClass(e, providers.ErrorPermissionDenied) || mutations != 0 {
		t.Fatal(e, mutations)
	}
	if d.owns(serverData{Name: d.name("identity") + "x"}) || d.name("a") == d.name("b") {
		t.Fatal("weak identity")
	}
}
func TestOVHClockSingleFlightAndCredentialValidation(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return response(strconv.FormatInt(time.Now().Unix(), 10)), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := d.client.clock(context.Background()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatal(calls)
	}
	for _, s := range []string{strings.Replace(fixtureCredential, "ovh-eu", "https://evil.invalid", 1), fixtureCredential + "{}", strings.Replace(fixtureCredential, "project_id", "wrong_field", 1)} {
		if _, e := credentials(context.Background(), testCredential(s)); e == nil || strings.Contains(e.Error(), "fixture-secret") {
			t.Fatal(e)
		}
	}
}
func TestOVHAmbiguousCreateFindsAllMatches(t *testing.T) {
	var d *Driver
	d = openTest(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/auth/time") {
			return response(readFixture(r.URL.Path)), nil
		}
		name := d.name("fixed")
		return response(`[{"id":"` + serverID + `","name":"` + name + `","region":"GRA11","status":"BUILD"},{"id":"44444444-4444-4444-4444-444444444444","name":"` + name + `","region":"GRA11","status":"BUILD"}]`), nil
	})
	xs, e := d.FindServerByIdentity(context.Background(), "fixed")
	if e != nil || len(xs) != 2 {
		t.Fatal("must expose duplicate ambiguity", xs, e)
	}
}
func TestOVHRejectsNonOfficialUbuntuNames(t *testing.T) {
	for _, name := range []string{"My ubuntu 24.04", "Ubuntu 24.04 ARM64", "Ubuntu 24.04 Desktop", "ubuntu 24.04"} {
		if usableImage(imageData{ID: imageID, Name: name, Type: "linux", User: "ubuntu", Region: "GRA11", Status: "active", Visibility: "public"}) {
			t.Fatal(name)
		}
	}
}

func TestOVHUnsupportedNetworkModeNeverCreates(t *testing.T) {
	for _, mode := range []string{"region-3-az", "localzone", ""} {
		t.Run(mode, func(t *testing.T) {
			mutations := 0
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" {
					mutations++
				}
				body := readFixture(r.URL.Path)
				if strings.HasSuffix(r.URL.Path, "/region/GRA11") {
					body = strings.Replace(body, `"type":"region"`, `"type":"`+mode+`"`, 1)
				}
				return response(body), nil
			})
			c, e := d.Catalog(context.Background())
			if mode == "" {
				if e == nil {
					t.Fatal("missing regional evidence accepted")
				}
			} else if e != nil || len(c.Regions) > 0 || len(c.Plans) != 1 || c.Plans[0].Available || len(c.Plans[0].IDsByRegion) != 0 || c.Plans[0].DetailsByRegion["GRA11"].Reason == "" || len(c.Images) > 0 {
				t.Fatal("unsupported mode advertised", c, e)
			}
			_, e = d.CreateServer(context.Background(), providers.CreateServerRequest{Identity: "fixed", RegionID: "GRA11", PlanID: flavorID, ImageID: imageID, SSHAuthorizedKeys: []string{key(t)}})
			if e == nil || mutations != 0 {
				t.Fatal("unsupported mode created", e, mutations)
			}
		})
	}
}
