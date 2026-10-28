package adminapi

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/kamatera"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/linode"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNewCloudProviderBrowser(t *testing.T) {
	if os.Getenv("DOB_RUN_BROWSER_TEST") != "1" {
		t.Skip("browser opt-in")
	}
	registry := providers.NewRegistry()
	for _, f := range []providers.Factory{digitalocean.Factory{}, vultr.Factory{}, upcloud.Factory{}, linode.Factory{}, aws.Factory{}, ovh.Factory{}, kamatera.Factory{}} {
		if e := registry.Register(f); e != nil {
			t.Fatal(e)
		}
	}
	s := Server{Container: app.Container{Providers: registry}}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../../web/static"))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "../../web/static/index.html") })
	mux.HandleFunc("GET /api/v1/providers", s.providersMetadata)
	// Match the authenticated panel bootstrap before exercising visible account controls.
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "isolated-browser-fixture", "role": "admin"})
	})
	var editFixture atomic.Bool
	mux.HandleFunc("POST /fixture/account-edit", func(w http.ResponseWriter, r *http.Request) { editFixture.Store(true); w.WriteHeader(204) })
	mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		if editFixture.Load() {
			writeJSON(w, 200, []any{map[string]any{"id": "kamatera-edit", "provider": "kamatera", "name": "Edit fixture", "regions": []string{"US-SC"}, "sizes": []string{"2B-4096-30"}, "image": "ubuntu26", "images": []string{"ubuntu26"}, "max_concurrent": 1, "settings_revision": 1}})
			return
		}
		writeJSON(w, 200, []any{})
	})
	mux.HandleFunc("GET /api/v1/proxies", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, []any{}) })

	var saveCount atomic.Int32
	var saveMode atomic.Value
	saveMode.Store("reject")
	mux.HandleFunc("POST /fixture/save-mode", func(w http.ResponseWriter, r *http.Request) {
		saveMode.Store(r.URL.Query().Get("mode"))
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /fixture/save-count", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, saveCount.Load()) })
	var starts atomic.Int32
	var approved atomic.Bool
	var emptyCatalog atomic.Bool
	var awsRejected atomic.Bool
	var activeID atomic.Value
	mux.HandleFunc("POST /api/v1/providers/ovh/authorizations", func(w http.ResponseWriter, r *http.Request) {
		var x struct {
			ID, Token string
			ProxyID   string `json:"proxy_id"`
		}
		if json.NewDecoder(r.Body).Decode(&x) != nil || !authorizationIDPattern.MatchString(x.ID) {
			t.Error("invalid authorization start")
			w.WriteHeader(400)
			return
		}
		var creds map[string]string
		json.Unmarshal([]byte(x.Token), &creds)
		if creds["application_secret"] != "fixture-secret" || creds["consumer_key"] != "" {
			t.Error("automatic flow requires only AK/AS")
			w.WriteHeader(400)
			return
		}
		starts.Add(1)
		activeID.Store(x.ID)
		// Simulate an issued credential whose response is lost. UI must observe, never repeat POST.
		writeJSON(w, 503, map[string]string{"error": "simulated_lost_response"})
	})
	mux.HandleFunc("GET /api/v1/provider-authorizations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if activeID.Load() != r.PathValue("id") {
			w.WriteHeader(404)
			return
		}
		state := "PENDING"
		if approved.Load() {
			state = "READY"
		}
		writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "provider": "ovh", "proxy_id": "", "state": state, "validation_url": "https://auth.eu.ovhcloud.com/sso/api?credentialToken=fixture-validation"})
	})
	mux.HandleFunc("POST /fixture/approve", func(w http.ResponseWriter, r *http.Request) { approved.Store(true); w.WriteHeader(204) })
	mux.HandleFunc("POST /fixture/catalog", func(w http.ResponseWriter, r *http.Request) {
		emptyCatalog.Store(r.URL.Query().Get("empty") == "1")
		w.WriteHeader(204)
	})
	var kamateraOptions atomic.Value
	mux.HandleFunc("GET /api/v1/accounts/kamatera-edit/options", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, kamateraOptions.Load()) })
	var previewMode atomic.Value
	previewMode.Store("")
	mux.HandleFunc("POST /fixture/preview-mode", func(w http.ResponseWriter, r *http.Request) {
		previewMode.Store(r.URL.Query().Get("mode"))
		w.WriteHeader(204)
	})
	previewFixture := func(w http.ResponseWriter, r *http.Request) {
		var x previewAccount
		if json.NewDecoder(r.Body).Decode(&x) != nil {
			t.Error("preview request invalid")
			w.WriteHeader(400)
			return
		}
		if x.Provider == "aws" && !awsRejected.Swap(true) {
			transport := quotaDiagnosticTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Query().Get("Action") == "GetCallerIdentity" {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<GetCallerIdentityResponse><GetCallerIdentityResult><Account>123456789012</Account><Arn>arn:aws:iam::123456789012:user/fixture</Arn><UserId>fixture</UserId></GetCallerIdentityResult></GetCallerIdentityResponse>`))}, nil
				}
				return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<Response><Errors><Error><Code>InvalidParameterValue</Code><Message>private-provider-message</Message></Error></Errors></Response>`)), Request: req}, nil
			})
			_, err := (aws.Factory{}).Discover(r.Context(), providers.OpenRequest{AccountID: "preview", Credentials: previewCredential{[]byte(x.Token)}, HTTPClient: &http.Client{Transport: transport}}, nil)
			if err == nil {
				t.Error("AWS fixture rejection accepted")
				w.WriteHeader(500)
				return
			}
			writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: "aws", Stage: "discovery", RequestID: "aws-browser-trace", Started: time.Now()})
			return
		}
		time.Sleep(100 * time.Millisecond)
		discovery := providers.Discovery{Complete: true, Resolved: map[string]any{}}
		region := "eu-west-1"
		providerID := "123456789012:" + region
		if x.Provider == "ovh" {
			if x.AuthorizationID == "" || x.Token != "" || activeID.Load() != x.AuthorizationID || !approved.Load() {
				t.Error("authorization not used for preview")
				w.WriteHeader(400)
				return
			}
			region = "GRA11"
			project := x.Selection["project_id"]
			providerID = "ovh-us:" + project
			discovery.Complete = project != ""
			discovery.Resolved["endpoint"] = "ovh-us"
			discovery.Fields = []providers.DiscoveryField{{Key: "project_id", Label: "Public Cloud project", Value: project, Options: []providers.DiscoveryOption{{Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "Project A"}, {Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Label: "Project B"}}}}
			if project != "" {
				discovery.Resolved["project_id"] = project
			}
		} else if x.Provider == "kamatera" {
			region = "US-SC"
			providerID = "kamatera-key:" + strings.Repeat("a", 64)
		} else {
			if x.Selection["region"] != "" {
				region = x.Selection["region"]
			}
			providerID = "123456789012:" + region
			primarySubnet := "subnet-12345678"
			if region != "eu-west-1" {
				primarySubnet = "subnet-11111111"
			}
			if x.Selection["subnet_id"] != "" && x.Selection["subnet_id"] != primarySubnet {
				t.Error("stale primary subnet reached preview")
			}
			discovery.Resolved = map[string]any{"region": region, "subnet_id": primarySubnet, "managed_network": true}
			discovery.Message = "Dedicated automatic security group: IPv4 TCP/UDP 1–65535; default outbound rules."
			regionOptions := []providers.DiscoveryOption{{Value: "eu-west-1", Label: "eu-west-1"}, {Value: "us-east-1", Label: "us-east-1"}, {Value: "us-west-1", Label: "us-west-1"}, {Value: "eu-west-2", Label: "eu-west-2"}, {Value: "eu-central-1", Label: "eu-central-1"}}
			discovery.Fields = []providers.DiscoveryField{{Key: "region", Label: "Primary AWS region", Value: region, Options: regionOptions}, {Key: "subnet_id", Label: "Verified public subnet", Value: primarySubnet, Options: []providers.DiscoveryOption{{Value: primarySubnet, Label: primarySubnet}}}}
			seenRegions := map[string]bool{region: true}
			for _, key := range []string{"region_2", "region_3", "region_4", "region_5"} {
				value := x.Selection[key]
				if value != "" && seenRegions[value] {
					t.Error("duplicate discovery region reached preview")
				}
				if value != "" {
					seenRegions[value] = true
				}
				discovery.Fields = append(discovery.Fields, providers.DiscoveryField{Key: key, Label: "Additional AWS region", Value: value, Options: append([]providers.DiscoveryOption{{Value: "", Label: "None"}}, regionOptions...)})
				if value != "" {
					subnetKey := "subnet_" + key[len(key)-1:]
					subnet := "subnet-87654321"
					if value != "us-east-1" {
						subnet = "subnet-22222222"
					}
					if x.Selection[subnetKey] != "" && x.Selection[subnetKey] != subnet {
						t.Error("stale additional subnet reached preview")
					}
					discovery.Fields = append(discovery.Fields, providers.DiscoveryField{Key: subnetKey, Label: "Public subnet", Value: subnet, Options: []providers.DiscoveryOption{{Value: subnet, Label: subnet}}})
				}
			}
		}
		if x.Provider == "aws" {
			discovery.NetworkApproval = true
			if x.Selection["region_2"] == "us-east-1" {
				discovery.Resolved["regional_networks"] = map[string]any{"us-east-1": map[string]any{"subnet_id": "subnet-87654321", "managed_network": true}}
			}
		}
		regions := []providers.Region{{ID: region, Name: region, Available: true}}
		plans := []providers.Plan{{ID: "test-plan", MemoryMB: 1024, CPU: 1, DiskGB: 20, Available: true}}
		images := []providers.Image{{ID: "ubuntu-fixture", Family: "ubuntu", Version: "24.04", Available: true}}
		var pricing *providers.CatalogPricing
		if x.Provider == "kamatera" {
			regions = append(regions, providers.Region{ID: "US-AT", Name: "Atlanta, United States", Available: true})
			plans = []providers.Plan{{ID: "1A-1024-20", MemoryMB: 1024, CPU: 1, DiskGB: 20, Available: true}, {ID: "2B-4096-30", MemoryMB: 4096, CPU: 2, DiskGB: 30, Available: true}}
			images = []providers.Image{{ID: "ubuntu26", Family: "ubuntu", Version: "26.04", Available: true}, {ID: "ubuntu24", Family: "ubuntu", Version: "24.04", Available: true}, {ID: "ubuntu22", Family: "ubuntu", Version: "22.04", Available: true}}
			pricing = &providers.CatalogPricing{Status: "partial", Source: "https://www.kamatera.com/pricing/", ObservedAt: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)}
			for i := range plans {
				typ, origin := "A", "custom_preset"
				if i == 1 {
					typ, origin = "B", "provider_preset"
				}
				plans[i].DetailsByRegion = map[string]providers.PlanRegionDetails{"US-SC": {CPUType: typ, CatalogOrigin: origin, BillingCycle: "hourly"}, "US-AT": {CPUType: typ, CatalogOrigin: origin, BillingCycle: "hourly"}}
				plans[i].PricesByRegion = map[string]providers.PlanPrice{"US-SC": {Currency: "USD", Hourly: float64([]int{4, 41}[i]) / 730, MonthlyEstimate: float64([]int{4, 41}[i]), Scope: "kamatera-public-base-hourly-v1", Complete: true}}
			}

		}
		if x.Provider == "aws" {
			plans[0].PricesByRegion = map[string]providers.PlanPrice{region: {Currency: "USD", Hourly: 0.0114, MonthlyEstimate: 8.322, Scope: "compute_only"}}
			if x.Selection["region_2"] == "us-east-1" {
				regions = append(regions, providers.Region{ID: "us-east-1", Name: "us-east-1", Available: true})
			}
		}
		if x.Provider == "ovh" {
			// Service-only SBG sorts before compute regions but has no plans.
			regions = append([]providers.Region{{ID: "SBG", Name: "Empty legacy region", Available: true}}, regions...)
			regions = append(regions, providers.Region{ID: "BHS5", Name: "BHS5", Available: true})
			plans[0].IDsByRegion = map[string]string{"GRA11": "f1", "BHS5": "f2"}
			plans[0].AvailableRegions = []string{"GRA11", "BHS5"}
			images[0].IDsByRegion = map[string]string{"GRA11": "i1"}
			images = append(images, providers.Image{ID: "ubuntu-bhs", Family: "ubuntu", Version: "22.04", Available: true, IDsByRegion: map[string]string{"BHS5": "i2"}})
			if emptyCatalog.Load() {
				plans = nil
			}
		}
		if x.Provider == "kamatera" {
			plans[1].IDsByRegion = map[string]string{"US-SC": plans[1].ID}
			kamateraOptions.Store(map[string]any{"plans": plans, "images": images, "regions": regions, "pricing": pricing})
		}
		writeJSON(w, 200, map[string]any{"provider": x.Provider, "onboarding": discovery, "account": providers.Account{ID: providerID}, "regions": regions, "plans": plans, "images": images, "pricing": pricing, "defaults": map[string]any{"region": "SBG"}, "policy": map[string]any{}})
	}
	mux.HandleFunc("POST /api/v1/accounts/preview", func(w http.ResponseWriter, r *http.Request) {
		mode := previewMode.Load().(string)
		if mode == "gateway" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(502)
			io.WriteString(w, "<html><title>502 Bad Gateway</title></html>")
			return
		}
		if mode == "truncated" {
			w.Header().Set("Content-Type", previewStreamType)
			io.WriteString(w, `{"type":"progress","stage":"catalog","elapsed_seconds":5}`+"\n")
			return
		}
		accountPreviewResponse(w, r, func(w http.ResponseWriter, r *http.Request) {
			previewStage(r.Context(), "catalog")
			if mode == "slow" {
				select {
				case <-time.After(350 * time.Millisecond):
				case <-r.Context().Done():
					return
				}
			}
			previewFixture(w, r)
		}, 5*time.Second, 40*time.Millisecond)
	})
	mux.HandleFunc("POST /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		var x accountWrite
		if json.NewDecoder(r.Body).Decode(&x) != nil {
			t.Error("save body invalid")
			w.WriteHeader(400)
			return
		}
		var credential map[string]any
		if x.Provider != "ovh" && json.Unmarshal([]byte(x.Token), &credential) != nil {
			t.Error("credential not JSON")
			w.WriteHeader(400)
			return
		}
		if x.Provider == "aws" {
			if credential["regional_networks"] == nil || credential["region"] != "eu-west-1" || credential["subnet_id"] != "subnet-12345678" || credential["managed_network"] != true || credential["secret_access_key"] != "fixture-secret" {
				t.Error("AWS save lost pinned selection or secret")
				w.WriteHeader(400)
				return
			}
		} else if x.Provider == "ovh" {
			if x.AuthorizationID == "" || x.Token != "" || activeID.Load() != x.AuthorizationID || starts.Load() != 1 || x.ExternalID != "ovh-us:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
				t.Error("OVH save lost pinned selection or secret")
				w.WriteHeader(400)
				return
			}
		} else if x.Provider == "kamatera" {
			saveCount.Add(1)
			mode := saveMode.Load().(string)
			if credential["client_id"] != "fixture-client" || credential["secret"] != "fixture-secret" || x.Name == "" {
				t.Error("Kamatera save lost required fields")
			}
			for _, values := range [][]string{x.Regions, x.Sizes, x.Images} {
				seen := map[string]bool{}
				for _, v := range values {
					if seen[v] {
						t.Error("duplicate preference reached save")
					}
					seen[v] = true
				}
			}
			time.Sleep(800 * time.Millisecond)
			if mode == "reject" {
				writeJSON(w, 422, map[string]string{"error": "fixture_save_rejected", "detail": "The fixture rejected this account. Entries are preserved."})
				return
			}
		} else {
			t.Error("wrong provider saved")
		}
		writeJSON(w, 201, map[string]string{"id": "fixture-account"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ready": true}) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: auth.Admin})))
	}))
	defer server.Close()
	root := os.Getenv("DOB_UI_ARTIFACT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, _ = filepath.Abs(root)
	os.MkdirAll(root, 0700)
	cmd := exec.Command("node", "../../web/tests/new-cloud-providers.cjs")
	cmd.Env = append(os.Environ(), "DOB_UI_BASE="+server.URL, "DOB_UI_ARTIFACT_DIR="+root)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("%v %s", e, out)
	}
	if starts.Load() != 1 {
		t.Fatal("authorization dispatched multiple times", starts.Load())
	}
	t.Log(string(out))
}
