package ovh

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type accessRule struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

var authorizationTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,512}$`)

func authorizationRules(project string) []accessRule {
	scope := "*"
	if project != "" {
		scope = project
	}
	base := "/cloud/project/" + scope
	rules := []accessRule{{"GET", "/auth/currentCredential"}, {"GET", base}, {"GET", base + "/*"}, {"POST", base + "/instance"}, {"DELETE", base + "/instance/*"}}
	if project == "" {
		rules = append([]accessRule{{"GET", "/cloud/project"}}, rules...)
	}
	return rules
}
func ValidAuthorizationURL(raw,endpoint string)bool{return validAuthorizationURL(raw,endpoint)}
func validAuthorizationURL(raw, endpoint string) bool {
	if len(raw) > 4096 {
		return false
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	// Explicit region/host/path pairs include the historical SDK URL and the
	// current official SSO route. No wildcard hosts or arbitrary redirect paths.
	routes := map[string]map[string][]string{
		"ovh-eu": {
			"eu.api.ovh.com":       {"/auth", "/auth/"},
			"api.ovh.com":          {"/auth", "/auth/"},
			"api.eu.ovhcloud.com":  {"/auth", "/auth/"},
			"www.ovh.com":          {"/auth/sso/api"},
			"auth.eu.ovhcloud.com": {"/sso/api", "/api/credential", "/api/credential/"},
		},
		"ovh-ca": {
			"ca.api.ovh.com":       {"/auth", "/auth/"},
			"api.ca.ovhcloud.com":  {"/auth", "/auth/"},
			"auth.ca.ovhcloud.com": {"/sso/api", "/api/credential", "/api/credential/"},
		},
		"ovh-us": {
			"api.us.ovhcloud.com":  {"/auth", "/auth/"},
			"auth.us.ovhcloud.com": {"/sso/api", "/api/credential", "/api/credential/"},
		},
	}
	allowed := false
	for _, path := range routes[endpoint][u.Host] {
		if u.Path == path {
			allowed = true
		}
	}
	// Keep a canonical single token query; reject encoded/duplicate keys,
	// redirect parameters and control characters in both server and browser.
	token := strings.TrimPrefix(u.RawQuery, "credentialToken=")
	return allowed && raw == "https://"+u.Host+u.Path+"?"+u.RawQuery && u.RawQuery == "credentialToken="+token && authorizationTokenPattern.MatchString(token)

}

// Only fixed local classifications are retained. Never expose response bodies,
// approval tokens, credentials or transport error text.
func authorizationUnknown(endpoint, code string, status int) error {
	return &requestError{cause: problem(providers.ErrorAmbiguousOutcome, "authorization", code, status), endpoint: endpoint, method: "POST", path: "/auth/credential"}
}
func (Factory) StartAuthorization(ctx context.Context, r providers.OpenRequest) (providers.AuthorizationGrant, error) {
	var cred credential
	if r.HTTPClient == nil || r.Credentials == nil || cloudutil.ReadCredential(ctx, r.Credentials, &cred) != nil ||
		!authorizationTokenPattern.MatchString(cred.ApplicationKey) || !authorizationTokenPattern.MatchString(cred.ApplicationSecret) ||
		cred.ConsumerKey != "" || (cred.Project != "" && !projectPattern.MatchString(cred.Project)) || (cred.Endpoint != "" && endpoints[cred.Endpoint] == "") {
		return providers.AuthorizationGrant{}, problem(providers.ErrorInvalidRequest, "authorization", "INVALID_CREDENTIAL_JSON", 0)
	}
	candidates := []string{cred.Endpoint}
	if cred.Endpoint == "" {
		candidates = []string{"ovh-eu", "ovh-ca", "ovh-us"}
	}
	h := cloudutil.HTTPClient(r.HTTPClient)
	h.Timeout = 8 * time.Second
	body, _ := json.Marshal(map[string]any{"accessRules": append(authorizationRules(cred.Project), accessRule{"GET", "/me"})})
	var last error
	for _, endpoint := range candidates {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoints[endpoint]+"/auth/credential", bytes.NewReader(body))
		req.Header.Set("X-Ovh-Application", cred.ApplicationKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, e := h.Do(req)
		if e != nil {
			return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_TRANSPORT_UNKNOWN", 0)
		}
		raw, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
		resp.Body.Close()
		if e != nil || len(raw) > 65536 {
			return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_BODY_UNKNOWN", resp.StatusCode)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			format, class := errorResponseShape(raw)
			last = &requestError{cause: classify("authorization", resp.StatusCode, raw), endpoint: endpoint, method: "POST", path: "/auth/credential", responseFormat: format, responseClass: class}
			// Only an explicit invalid application key proves no credential was issued.
			if Diagnostic(last).Code == "INVALID_KEY" {
				continue
			}
			if resp.StatusCode >= 500 {
				return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_SERVER_UNKNOWN", resp.StatusCode)
			}
			if resp.StatusCode < 400 {
				return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_REDIRECT_UNKNOWN", resp.StatusCode)
			}
			return providers.AuthorizationGrant{}, last
		}
		var out struct {
			State         string `json:"state"`
			ConsumerKey   string `json:"consumerKey"`
			ValidationURL string `json:"validationUrl"`
		}
		if json.Unmarshal(raw, &out) != nil {
			return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_JSON_UNKNOWN", resp.StatusCode)
		}
		if out.State != "pendingValidation" || !authorizationTokenPattern.MatchString(out.ConsumerKey) {
			return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_GRANT_UNKNOWN", resp.StatusCode)
		}
		if !validAuthorizationURL(out.ValidationURL, endpoint) {
			return providers.AuthorizationGrant{}, authorizationUnknown(endpoint, "AUTHORIZATION_URL_UNKNOWN", resp.StatusCode)
		}
		cred.Endpoint = endpoint
		cred.ConsumerKey = out.ConsumerKey
		return providers.AuthorizationGrant{Credentials: cloudutil.NewJSONCredential(cred), ValidationURL: out.ValidationURL}, nil
	}
	return providers.AuthorizationGrant{}, last
}
func (Factory) CheckAuthorization(ctx context.Context, r providers.OpenRequest) (bool, error) {
	if r.HTTPClient == nil || r.Credentials == nil {
		return false, problem(providers.ErrorInvalidRequest, "authorization", "INVALID_CREDENTIAL_JSON", 0)
	}
	cred, e := signingCredentials(ctx, r.Credentials)
	if e != nil {
		return false, e
	}
	c := &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: r.Credentials, endpoint: cred.Endpoint, project: cred.Project}
	var out struct {
		Status     string       `json:"status"`
		Rules      []accessRule `json:"rules"`
		Expiration *time.Time   `json:"expiration"`
	}
	e = c.do(ctx, http.MethodGet, "/auth/currentCredential", nil, &out)
	if e != nil {
		code := Diagnostic(e).Code
		if code == "INVALID_CREDENTIAL" || code == "NOT_CREDENTIAL" {
			return false, nil
		}
		return false, e
	}
	switch out.Status {
	case "pendingValidation":
		return false, nil
	case "refused", "expired":
		return false, problem(providers.ErrorPermissionDenied, "authorization", "AUTHORIZATION_REFUSED_OR_EXPIRED", 403)
	case "validated":
	default:
		return false, problem(providers.ErrorUnavailable, "authorization", "INVALID_RESPONSE", 200)
	}
	if out.Expiration != nil && !out.Expiration.After(time.Now()) {
		return false, problem(providers.ErrorAuthentication, "authorization", "AUTHORIZATION_REFUSED_OR_EXPIRED", 401)
	}
	// The authorization screen may reduce permissions. Check the actual grant,
	// including future create/delete rights, without creating paid resources.
	for _, wanted := range authorizationRules(cred.Project) {
		found := false
		for _, got := range out.Rules {
			if strings.EqualFold(got.Method, wanted.Method) && got.Path == wanted.Path {
				found = true
			}
		}
		if !found {
			return false, problem(providers.ErrorPermissionDenied, "authorization", "AUTHORIZATION_RIGHTS_INCOMPLETE", 403)
		}
	}
	return true, nil
}
