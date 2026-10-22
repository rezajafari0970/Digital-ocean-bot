package ovh

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"net/http"
	"sort"
	"time"
)

func (Factory) Discover(ctx context.Context, r providers.OpenRequest, selection map[string]string) (providers.Discovery, error) {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	var cred credential
	if cloudutil.ReadCredential(ctx, r.Credentials, &cred) != nil {
		return providers.Discovery{}, problem(providers.ErrorAuthentication, "discovery", "INVALID_CREDENTIAL_JSON", 0)
	}
	if selection["endpoint"] != "" {
		cred.Endpoint = selection["endpoint"]
	}
	if selection["project_id"] != "" {
		cred.Project = selection["project_id"]
	}
	if cred.Project != "" && !projectPattern.MatchString(cred.Project) {
		return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "INVALID_PROJECT_ID", 0)
	}
	candidates := []string{cred.Endpoint}
	if cred.Endpoint == "" {
		candidates = []string{"ovh-eu", "ovh-ca", "ovh-us"}
	}
	var c *client
	var ids []string
	var last error
	for _, endpoint := range candidates {
		candidate := cred
		candidate.Endpoint = endpoint
		source := cloudutil.NewJSONCredential(candidate)
		if _, e := signingCredentials(ctx, source); e != nil {
			return providers.Discovery{}, e
		}
		current := &client{http: cloudutil.HTTPClient(r.HTTPClient), credentials: source, endpoint: endpoint, project: candidate.Project}
		if candidate.Project != "" {
			// Explicit project credentials may deliberately lack permission to list all projects.
			var p struct {
				ID     string `json:"project_id"`
				Status string `json:"status"`
			}
			last = current.do(ctx, http.MethodGet, current.path(""), nil, &p)
			if last == nil {
				if p.ID != candidate.Project {
					return providers.Discovery{}, problem(providers.ErrorUnavailable, "discovery", "PROJECT_ID_MISMATCH", 200)
				}
				ids = []string{p.ID}
			}
		} else {
			ids, last = list(ctx, current, "/cloud/project", func(s string) string {
				if !projectPattern.MatchString(s) {
					return ""
				}
				return s
			})
		}
		if last == nil {
			c = current
			cred = candidate
			break
		}
		if len(candidates) == 1 {
			return providers.Discovery{}, last
		}
		// A missing access rule belongs to this endpoint. Never overwrite it
		// with INVALID_KEY from another region. Only INVALID_KEY establishes
		// that the application is not recognized at the current endpoint.
		if Diagnostic(last).Code != "INVALID_KEY" {
			return providers.Discovery{}, last
		}
	}
	if c == nil {
		return providers.Discovery{}, last
	}
	sort.Strings(ids)
	options := []providers.DiscoveryOption{}
	for _, id := range ids {
		var p struct {
			ID          string `json:"project_id"`
			Description string `json:"description"`
			Status      string `json:"status"`
		}
		if e := c.do(ctx, http.MethodGet, "/cloud/project/"+id, nil, &p); e != nil {
			return providers.Discovery{}, e
		}
		if p.ID != id {
			return providers.Discovery{}, problem(providers.ErrorUnavailable, "discovery", "PROJECT_ID_MISMATCH", 200)
		}
		if p.Status == "ok" {
			label := id
			if p.Description != "" {
				label = p.Description + " (" + id + ")"
			}
			options = append(options, providers.DiscoveryOption{Value: id, Label: label})
		}
	}
	if len(options) == 0 {
		return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "NO_ACTIVE_PUBLIC_CLOUD_PROJECT", 0)
	}
	if cred.Project == "" && len(options) == 1 {
		cred.Project = options[0].Value
	}
	selected := false
	for _, o := range options {
		if o.Value == cred.Project {
			selected = true
		}
	}
	if cred.Project != "" && !selected {
		return providers.Discovery{}, problem(providers.ErrorInvalidRequest, "discovery", "PROJECT_NOT_ACTIVE_OR_ACCESSIBLE", 0)
	}
	resolved := map[string]any{"endpoint": cred.Endpoint}
	if selected {
		resolved["project_id"] = cred.Project
	}
	message := "Endpoint detected: " + cred.Endpoint + ". Select the Public Cloud project to use."
	if selected {
		message = "Endpoint and active Public Cloud project verified: " + cred.Endpoint + " / " + cred.Project
	}
	return providers.Discovery{Complete: selected, Fields: []providers.DiscoveryField{{Key: "project_id", Label: "Public Cloud project", Value: cred.Project, Options: options}}, Resolved: resolved, Message: message, Credentials: cloudutil.NewJSONCredential(cred)}, nil
}
