package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"sort"
	"strings"
	"time"
)

type previewAccount struct {
	AuthorizationID string            `json:"authorization_id,omitempty"`
	Selection       map[string]string `json:"selection,omitempty"`
	Provider        string            `json:"provider"`
	Token           string            `json:"token"`
	ProxyID         string            `json:"proxy_id"`
}
type previewCredential struct{ token []byte }

func (m previewCredential) Get(context.Context) ([]byte, error) {
	return append([]byte(nil), m.token...), nil
}

func (s *Server) runAccountPreview(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var randomID [8]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	previewID := hex.EncodeToString(randomID[:])
	started := time.Now()
	var x previewAccount
	previewExitIP := ""
	fail := func(err error, stage string) {
		writeStagedProviderPreviewError(w, err, previewDiagnosticContext{Provider: x.Provider, Stage: stage, RequestID: previewID, Started: started, ExitIP: previewExitIP})
	}
	if json.NewDecoder(r.Body).Decode(&x) != nil || (strings.TrimSpace(x.Token) == "" && x.AuthorizationID == "") {
		writeJSON(w, 400, map[string]string{"error": "token_required"})
		return
	}
	if x.Provider == "" {
		x.Provider = "digitalocean"
	}
	if s.Container.Providers == nil || !s.Container.Providers.Has(x.Provider) {
		writeJSON(w, 400, map[string]string{"error": "provider_not_supported"})
		return
	}
	var flow authorizationRecord
	var flowSecret authorizationSecret
	if x.AuthorizationID != "" {
		if x.Token != "" {
			writeJSON(w, 400, map[string]string{"error": "authorization_token_conflict"})
			return
		}
		unlock, ok := s.authorizationLock(w, r, x.AuthorizationID)
		if !ok {
			return
		}
		defer unlock()
		var e error
		flow, flowSecret, e = s.authorizationCredential(r.Context(), x.AuthorizationID, x.Provider, x.ProxyID, false)
		if e != nil {
			writeJSON(w, 409, map[string]string{"error": "authorization_not_ready"})
			return
		}
		x.Token = flowSecret.Token
		var scope struct {
			Endpoint string `json:"endpoint"`
			Project  string `json:"project_id"`
		}
		if json.Unmarshal([]byte(x.Token), &scope) != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if (x.Selection["endpoint"] != "" && x.Selection["endpoint"] != scope.Endpoint) || (scope.Project != "" && x.Selection["project_id"] != "" && x.Selection["project_id"] != scope.Project) {
			writeJSON(w, 409, map[string]string{"error": "authorization_scope_changed"})
			return
		}
	}
	previewStage(r.Context(), "connection")
	client, closeFn, observed, ok := s.previewClient(w, r, x.ProxyID, true)
	if !ok {
		return
	}
	defer closeFn()
	previewExitIP = observed

	request := providers.OpenRequest{AccountID: "preview", HTTPClient: client, Credentials: previewCredential{[]byte(x.Token)}}
	previewStage(r.Context(), "discovery")
	discovery, err := s.Container.Providers.Discover(r.Context(), x.Provider, request, x.Selection)
	if err != nil {
		fail(err, "discovery")
		return
	}
	if !discovery.Complete {
		writeJSON(w, 200, map[string]any{"provider": x.Provider, "onboarding": discovery})
		return
	}
	request.Credentials = discovery.Credentials
	previewStage(r.Context(), "credential")
	driver, err := s.Container.Providers.Open(r.Context(), x.Provider, request)
	if err != nil {
		if x.Provider == "ovh" || x.Provider == "aws" || x.Provider == "kamatera" {
			fail(err, "credential")
			return
		}
		writeJSON(w, 422, map[string]string{"error": "provider_validation_failed", "detail": err.Error()})
		return
	}
	ar, aok := driver.(providers.AccountReader)
	cr, cok := driver.(providers.CatalogReader)
	if !aok || !cok {
		writeJSON(w, 422, map[string]string{"error": "provider_capability_missing"})
		return
	}
	previewStage(r.Context(), "account")
	account, err := ar.Account(r.Context())
	if err != nil {
		fail(err, "account")
		return
	}
	previewStage(r.Context(), "capacity")
	capacity, capErr := ar.Capacity(r.Context())
	if capErr != nil {
		fail(capErr, "capacity")
		return
	}
	previewStage(r.Context(), "catalog")
	catalog, err := cr.Catalog(r.Context())
	if err != nil {
		fail(err, "catalog")
		return
	}
	// Authentication alone does not make an account ready for server setup.
	// Apply the same supported OS policy as the account form before verification.
	setupCatalog := catalog
	setupCatalog.Images = nil
	for _, im := range catalog.Images {
		if im.Family == "ubuntu" && (im.Version == "22.04" || im.Version == "24.04" || im.Version == "26.04") {
			setupCatalog.Images = append(setupCatalog.Images, im)
		}
	}
	regions := providers.CatalogRegionsWithOptions(setupCatalog)
	if len(regions) == 0 {
		writeJSON(w, 422, map[string]any{"error": "provider_catalog_unavailable", "stage": "catalog", "request_id": previewID, "detail": "Provider access is verified, but no region currently has both an available supported plan and Ubuntu image. Check regional availability in the provider account, then reload provider options."})
		return
	}
	sort.SliceStable(regions, func(i, j int) bool { return regions[i].Name < regions[j].Name })
	images := catalog.Images
	sort.SliceStable(images, func(i, j int) bool {
		ui := images[i].Family == "ubuntu"
		uj := images[j].Family == "ubuntu"
		if ui != uj {
			return ui
		}
		return images[i].Version > images[j].Version
	})
	proxies := []map[string]any{{"id": "", "name": "No proxy — Direct server IP", "status": "direct"}}
	rows, queryErr := s.DB.QueryContext(r.Context(), `SELECT id::text,name,status,COALESCE(country,'') FROM proxies WHERE status='healthy' ORDER BY name`)
	if queryErr != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody())
		return
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, n, st, c string
			if rows.Scan(&id, &n, &st, &c) == nil {
				proxies = append(proxies, map[string]any{"id": id, "name": n, "status": st, "country": c})
			} else {
				rows.Close()
				writeJSON(w, http.StatusInternalServerError, errorBody())
				return
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			writeJSON(w, http.StatusInternalServerError, errorBody())
			return
		}
	}
	if x.AuthorizationID != "" {
		raw, e := discovery.Credentials.Get(r.Context())
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		defer zeroBytes(raw)
		flowSecret.PreparedToken = string(raw)
		if e = s.storeAuthorizationSecret(r.Context(), flow, "VERIFIED", flowSecret, account.ID); e != nil {
			writeJSON(w, 409, map[string]string{"error": "authorization_no_longer_active"})
			return
		}
	}
	meta, _ := s.Container.Providers.Metadata(driver.Name())
	writeJSON(w, 200, map[string]any{"provider": driver.Name(), "onboarding": discovery, "policy": meta, "account": account, "server_limit": capacity.ComputeLimit, "provider_servers": capacity.ComputeInUse, "limit_known": capacity.LimitKnown, "capacity_source": capacity.Source, "regions": regions, "pricing": catalog.Pricing, "plans": catalog.Plans, "sizes": catalog.Plans, "images": images, "proxies": proxies, "defaults": map[string]any{"interval_seconds": 300, "batch_size": 1, "max_concurrent": 1}})
}
func writeProviderPreviewError(w http.ResponseWriter, err error, exitIP string) {
	switch providers.Class(err) {
	case providers.ErrorAuthentication:
		detail := "Provider rejected these credentials."
		if exitIP != "" {
			detail += " Proxy exit IP: " + exitIP
		}
		writeJSON(w, 401, map[string]string{"error": "provider_credential_invalid", "detail": detail, "exit_ip": exitIP})
	case providers.ErrorPermissionDenied, providers.ErrorAccountLocked:
		writeJSON(w, 403, map[string]string{"error": "provider_permission_denied", "detail": err.Error()})
	case providers.ErrorRateLimited:
		writeJSON(w, 429, map[string]string{"error": "provider_rate_limited", "detail": "Provider rate limit reached. Try again later."})
	case providers.ErrorTransport, providers.ErrorUnavailable:
		writeJSON(w, 503, map[string]string{"error": "provider_transport_failed", "detail": "Could not reach provider through the selected connection."})
	default:
		writeJSON(w, 422, map[string]string{"error": "provider_validation_failed", "detail": err.Error()})
	}
}
