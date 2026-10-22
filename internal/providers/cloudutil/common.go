// Package cloudutil contains stateless helpers. Credentials, transports and
// lifecycle decisions remain owned by each independent provider driver.
package cloudutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
)

func HTTPClient(h *http.Client) *http.Client {
	c := *h
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if c.Timeout <= 0 || c.Timeout > 30*time.Second {
		c.Timeout = 30 * time.Second
	}
	return &c
}
func Problem(provider string, class providers.ErrorClass, op, code string, status int) *providers.Error {
	return &providers.Error{Class: class, Operation: op, Code: code, StatusCode: status, Message: provider + ": " + code}
}
func ReadCredential(ctx context.Context, source providers.CredentialSource, out any) error {
	b, e := source.Get(ctx)
	if e != nil {
		return e
	}
	defer func() {
		for i := range b {
			b[i] = 0
		}
	}()
	if len(b) == 0 || len(b) > 32768 {
		return errors.New("invalid credential")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return errors.New("invalid credential")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("invalid credential")
	}
	return nil
}
func Defaults() providers.Defaults {
	return providers.Defaults{Images: providers.ImagePolicy{Family: "ubuntu", Versions: []string{"26.04", "24.04", "22.04"}}, LifetimeMinMinutes: 90, LifetimeMaxMinutes: 120, DesiredServers: 1, BuildSpacingMinMinutes: 1, BuildSpacingMaxMinutes: 3, MaxConcurrent: 1, FallbackAnyRegion: true}
}
func PublicIPv4(s string) string {
	a, e := netip.ParseAddr(s)
	if e != nil || !a.Is4() || !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return ""
	}
	return a.String()
}
func Contains(xs []string, v string) bool {
	for _, s := range xs {
		if s == v {
			return true
		}
	}
	return false
}
func Ambiguous(e error) bool {
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return true
	}
	switch providers.Class(e) {
	case providers.ErrorTransport, providers.ErrorUnavailable, providers.ErrorUnknown, providers.ErrorAmbiguousOutcome:
		return true
	}
	return false
}
func CloudInit(keys []string) (string, error) {
	if len(keys) == 0 || len(keys) > 8 {
		return "", errors.New("SSH key required")
	}
	clean := []string{}
	for _, s := range keys {
		k, _, opts, rest, e := ssh.ParseAuthorizedKey([]byte(s))
		if e != nil || len(opts) > 0 || len(bytes.TrimSpace(rest)) > 0 {
			return "", errors.New("invalid SSH key")
		}
		clean = append(clean, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k))))
	}
	// JSON is valid YAML; never interpolate a public key into a shell command.
	b, e := json.Marshal(map[string]any{"disable_root": false, "ssh_pwauth": false, "users": []any{"default", map[string]any{"name": "root", "lock_passwd": true, "ssh_authorized_keys": clean}}, "write_files": []any{map[string]any{"path": "/etc/ssh/sshd_config.d/00-dob-key-only.conf", "permissions": "0600", "content": "PermitRootLogin prohibit-password\nPasswordAuthentication no\nKbdInteractiveAuthentication no\n"}}, "runcmd": []any{[]string{"systemctl", "reload", "ssh"}}})
	return "#cloud-config\n" + string(b) + "\n", e
}
func Version(name string) string {
	name = strings.ToLower(name)
	if !strings.Contains(name, "ubuntu") || strings.Contains(name, "arm") || strings.Contains(name, "minimal") || strings.Contains(name, "pro-") {
		return ""
	}
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		if strings.Contains(name, v) {
			return v
		}
	}
	return ""
}
