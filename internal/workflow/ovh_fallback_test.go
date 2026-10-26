package workflow

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"golang.org/x/crypto/ssh"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type regionalTestCredential string

func (c regionalTestCredential) Get(context.Context) ([]byte, error) { return []byte(c), nil }

type regionalTestTransport func(*http.Request) (*http.Response, error)

func (f regionalTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type regionalTestGate struct{}

func (regionalTestGate) AllowMutation() error { return nil }

type regionalOperationStore struct{ ops map[string]jobs.Operation }

func (s *regionalOperationStore) Reserve(_ context.Context, o jobs.Operation) (jobs.Operation, bool, error) {
	if x, ok := s.ops[o.IdempotencyKey]; ok {
		return x, false, nil
	}
	o.ID = o.IdempotencyKey
	o.State = jobs.OperationPlanned
	s.ops[o.IdempotencyKey] = o
	return o, true, nil
}
func (s *regionalOperationStore) Get(_ context.Context, account, key string) (jobs.Operation, error) {
	o, ok := s.ops[key]
	if !ok || o.AccountID != account {
		return o, jobs.ErrOperationNotFound
	}
	return o, nil
}
func (s *regionalOperationStore) Update(_ context.Context, o *jobs.Operation) error {
	s.ops[o.IdempotencyKey] = *o
	return nil
}

func TestOVHQuotaFallbackUsesOnlyApprovedRegionsAndRecoversJournal(t *testing.T) {
	const plan = "22222222-2222-2222-2222-222222222222"
	const image = "33333333-3333-3333-3333-333333333333"
	const server = "11111111-1111-1111-1111-111111111111"
	for _, tc := range []struct {
		name                string
		flavor, allow, lost bool
	}{
		{"flavor-quota", true, true, false}, {"regional-quota", false, true, false}, {"fallback-disabled", true, false, false}, {"lost-create-response", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			regions := []string{"BHS5"}
			if tc.allow {
				regions = append(regions, "GRA11")
			}
			tr := regionalTestTransport(func(r *http.Request) (*http.Response, error) {
				reply := func(body string) *http.Response {
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/time"):
					return reply(strconv.FormatInt(time.Now().Unix(), 10)), nil
				case strings.Contains(r.URL.Path, "/region/"):
					rg := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
					return reply(`{"name":"` + rg + `","status":"UP","type":"region"}`), nil
				case strings.HasSuffix(r.URL.Path, "/flavor"):
					rg := r.URL.Query().Get("region")
					quota := "1"
					if rg == "BHS5" && tc.flavor {
						quota = "0"
					}
					return reply(`[{"id":"` + plan + `","name":"d2-4","region":"` + rg + `","osType":"linux","type":"ovh.d2","vcpus":2,"ram":4,"disk":50,"available":true,"quota":` + quota + `}]`), nil
				case strings.HasSuffix(r.URL.Path, "/image"):
					return reply(`[{"id":"` + image + `","name":"Ubuntu 24.04","type":"linux","user":"ubuntu","region":"` + r.URL.Query().Get("region") + `","status":"active","visibility":"public","minRam":1024,"minDisk":10}]`), nil
				case strings.HasSuffix(r.URL.Path, "/quota"):
					return reply(`[{"region":"BHS5","instance":{"maxCores":6,"usedCores":6,"maxInstances":3,"usedInstances":3,"maxRam":25000,"usedRAM":12000}},{"region":"GRA11","instance":{"maxCores":6,"usedCores":0,"maxInstances":3,"usedInstances":0,"maxRam":25000,"usedRAM":0}}]`), nil
				case strings.HasSuffix(r.URL.Path, "/instance") && r.Method == "POST":
					posts++
					var b map[string]any
					json.NewDecoder(r.Body).Decode(&b)
					if b["region"] != "GRA11" {
						t.Fatal("mutation in exhausted or unapproved region")
					}
					if tc.lost {
						return nil, io.ErrUnexpectedEOF
					}
					out, _ := json.Marshal(map[string]any{"id": server, "name": b["name"], "region": b["region"], "monthlyBilling": nil})
					return reply(string(out)), nil
				case strings.HasSuffix(r.URL.Path, "/instance"):
					return reply("[]"), nil
				default:
					t.Fatal("unexpected request", r.Method, r.URL.Path)
					return nil, errors.New("unexpected")
				}
			})
			driver, e := (ovh.Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "account-fixture", Credentials: regionalTestCredential(`{"endpoint":"ovh-eu","application_key":"fixture","application_secret":"fixture","consumer_key":"fixture","project_id":"0123456789abcdef0123456789abcdef"}`), HTTPClient: &http.Client{Transport: tr}})
			if e != nil {
				t.Fatal(e)
			}
			pub, _, _ := ed25519.GenerateKey(rand.Reader)
			key, _ := ssh.NewPublicKey(pub)
			store := &regionalOperationStore{ops: map[string]jobs.Operation{}}
			runtime := RuntimeSteps{Droplets: droplets.Executor{Provider: driver.(providers.ComputeDriver), Operations: store, Gate: regionalTestGate{}}, Profile: droplets.Profile{Name: "fixture", Region: "BHS5", Regions: regions, Size: plan, Image: image, SSHAuthorizedKey: string(ssh.MarshalAuthorizedKey(key)), RegionalLaunches: map[string]providers.RegionalLaunch{"BHS5": {PlanID: plan, ImageID: image}, "GRA11": {PlanID: plan, ImageID: image}}}}
			dep := Deployment{ID: "22222222-2222-4222-8222-222222222222", AccountID: "account-fixture"}
			result, e := runtime.Create(context.Background(), dep)
			if !tc.allow {
				if posts != 0 || !providers.IsClass(e, providers.ErrorRegionCapacity) {
					t.Fatal(posts, e)
				}
				return
			}
			if tc.lost {
				if posts != 1 || !providers.IsClass(e, providers.ErrorAmbiguousOutcome) {
					t.Fatal(posts, e)
				}
			} else if e != nil || posts != 1 || result.ProviderID != server {
				t.Fatal(posts, e, result)
			}
			// Resume after an interrupted workflow checkpoint: the first region's failed
			// operation must not strand the already accepted/uncertain second region.
			result, e = runtime.Create(context.Background(), dep)
			if posts != 1 {
				t.Fatal("replayed a billable create", posts)
			}
			if tc.lost {
				if e == nil {
					t.Fatal("uncertain outcome accepted")
				}
			} else if e != nil || result.ProviderID != server {
				t.Fatal("journal replay blocked fallback", e, result)
			}
		})
	}
}
