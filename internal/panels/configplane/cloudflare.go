package configplane

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"time"
)

func (s Service) cloudflare() cloudflare.Service {
	secrets, _ := s.Secrets.(cloudflare.TokenStore)
	return cloudflare.Service{DB: s.DB, Secrets: secrets}
}
func (s Service) ensureCloudflareCertificate(ctx context.Context, p Profile, panel string, t target, key []byte) (time.Time, error) {
	node, ok := s.SSH.(cloudflare.Node)
	if !ok {
		return time.Time{}, errors.New("profile_cloudflare_node_unavailable")
	}
	if ssh, ok := s.SSH.(provisioning.SSHClient); ok {
		ssh.OutputLimit = 65536
		node = ssh
	}
	until, e := s.cloudflare().EnsureCertificate(ctx, p.Spec.CloudflareAccount, panel, node, t.Target, key)
	if e != nil {
		return time.Time{}, errors.New("profile_" + cloudflare.ErrorCode(e))
	}
	return until, nil
}

// Cached expiry is evidence only for the certificate driver that produced it.
func needsPublicIPCertificate(desired, applied configdriver.Spec, cert sql.NullTime, now time.Time) bool {
	return desired.Security == "tls" && desired.CloudflareAccount == "" && (applied.Security != "tls" || applied.CloudflareAccount != "" || !cert.Valid || cert.Time.Before(now.Add(48*time.Hour)))
}
