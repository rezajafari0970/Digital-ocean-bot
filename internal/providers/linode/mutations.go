package linode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

var labelPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{1,62}[A-Za-z0-9]$`)

func identityTag(identity string) string {
	if strings.HasPrefix(identity, "dob-deployment-") {
		return "dob-dep-" + strings.TrimPrefix(identity, "dob-deployment-")
	}
	if utf8.RuneCountInString(identity) <= 50 {
		return identity
	}
	sum := sha256.Sum256([]byte(identity))
	return "dob-id-" + hex.EncodeToString(sum[:20])
}
func (d *Driver) ownerTag() string { return "dob-account-" + d.accountID }
func (d *Driver) owns(x serverData) bool {
	return contains(x.Tags, d.ownerTag()) && contains(x.Tags, "managed-by-digital-ocean-bot")
}

type createRequest struct {
	Label          string   `json:"label"`
	Region         string   `json:"region"`
	Type           string   `json:"type"`
	Image          string   `json:"image"`
	AuthorizedKeys []string `json:"authorized_keys"`
	Tags           []string `json:"tags"`
	Booted         bool     `json:"booted"`
	Backups        bool     `json:"backups_enabled"`
	PrivateIP      bool     `json:"private_ip"`
}

func (d *Driver) CreateServer(ctx context.Context, req providers.CreateServerRequest) (providers.CreateServerResult, error) {
	rejected := providers.CreateServerResult{Outcome: providers.OutcomeRejected}
	invalid := func(code string) (providers.CreateServerResult, error) {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", code, 0)
	}
	if !labelPattern.MatchString(req.Name) || strings.Contains(req.Name, "--") || strings.Contains(req.Name, "__") || strings.Contains(req.Name, "..") {
		return invalid("INVALID_LABEL")
	}
	if req.RegionID == "" || req.PlanID == "" || req.Identity == "" || len(req.SSHAuthorizedKeys) == 0 || len(req.SSHKeyRefs) != 0 {
		return invalid("INCOMPLETE_CREATE_REQUEST")
	}
	validImage := false
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		validImage = validImage || req.ImageID == "linode/ubuntu"+v
	}
	if !validImage {
		return invalid("UNSUPPORTED_IMAGE")
	}
	keys := []string{}
	for _, raw := range req.SSHAuthorizedKeys {
		key, _, opts, rest, e := ssh.ParseAuthorizedKey([]byte(raw))
		if e != nil || len(opts) > 0 || len(bytes.TrimSpace(rest)) > 0 {
			return invalid("INVALID_SSH_PUBLIC_KEY")
		}
		keys = append(keys, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
	}
	tags := []string{"managed-by-digital-ocean-bot", d.ownerTag(), identityTag(req.Identity)}
	for _, tag := range req.Tags {
		if !contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	for _, tag := range tags {
		if utf8.RuneCountInString(tag) < 3 || utf8.RuneCountInString(tag) > 50 || strings.ContainsAny(tag, "\r\n\x00") {
			return invalid("INVALID_TAG")
		}
	}
	if e := d.requireFullAccess(ctx); e != nil {
		return rejected, e
	}
	// Account-wide automatic backups override backups_enabled:false. Do not silently
	// create additional paid services or modify account-wide policy.
	var settings struct {
		Backups *bool `json:"backups_enabled"`
		Managed *bool `json:"managed"`
	}
	if e := d.client.do(ctx, http.MethodGet, "/account/settings", nil, &settings); e != nil {
		return rejected, e
	}
	if settings.Backups == nil || settings.Managed == nil {
		return rejected, problem(providers.ErrorUnavailable, "create_server", "MISSING_ACCOUNT_SETTINGS", 200)
	}
	if *settings.Backups || *settings.Managed {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", "ACCOUNT_PAID_ADDONS_ENABLED", 0)
	}
	var x serverData
	e := d.client.do(ctx, http.MethodPost, "/linode/instances", createRequest{Label: req.Name, Region: req.RegionID, Type: req.PlanID, Image: req.ImageID, AuthorizedKeys: keys, Tags: tags, Booted: true}, &x)
	if e != nil {
		if isAmbiguous(e) {
			return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_OUTCOME_UNCONFIRMED", 0)
		}
		return rejected, e
	}
	if serverID(x) == "" {
		return providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}, problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_ID_MISSING", 200)
	}
	return providers.CreateServerResult{ServerID: serverID(x), Outcome: providers.OutcomeAccepted}, nil
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	x, e := d.rawServer(ctx, id)
	if providers.IsClass(e, providers.ErrorNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if !d.owns(x) {
		return problem(providers.ErrorPermissionDenied, "delete_server", "OWNERSHIP_NOT_PROVEN", 0)
	}
	// Local disks/backups are removed by this endpoint. This driver creates no
	// external volumes or reserved IPs and never deletes unrelated resources.
	e = d.client.do(ctx, http.MethodDelete, "/linode/instances/"+id, nil, nil)
	if providers.IsClass(e, providers.ErrorNotFound) {
		return nil
	}
	return e
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	if identity == "" {
		return nil, problem(providers.ErrorInvalidRequest, "find_server", "MISSING_IDENTITY", 0)
	}
	xs, e := d.ListServers(ctx)
	if e != nil {
		return nil, e
	}
	out := []providers.Server{}
	for _, x := range xs {
		if contains(x.Tags, identityTag(identity)) && contains(x.Tags, d.ownerTag()) && contains(x.Tags, "managed-by-digital-ocean-bot") {
			out = append(out, x)
		}
	}
	return out, nil
}
