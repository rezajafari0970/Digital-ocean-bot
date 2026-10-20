package configplane

import (
	"database/sql"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"testing"
	"time"
)

func TestCloudflareCertificateDriverTransition(t *testing.T) {
	now := time.Now()
	valid := sql.NullTime{Time: now.Add(7 * 24 * time.Hour), Valid: true}
	public := configdriver.Spec{Security: "tls"}
	cdn := public
	cdn.CloudflareAccount = "owned"
	for _, x := range []struct {
		name             string
		desired, applied configdriver.Spec
		cert             sql.NullTime
		want             bool
	}{
		{"retained public certificate", public, public, valid, false},
		{"Origin CA cannot prove public IP TLS", public, cdn, valid, true},
		{"enabling CDN uses its own certificate driver", cdn, public, valid, false},
		{"old non-TLS cache", public, configdriver.Spec{Security: "none"}, valid, true},
		{"missing certificate", public, public, sql.NullTime{}, true},
		{"renewal margin", public, public, sql.NullTime{Time: now.Add(24 * time.Hour), Valid: true}, true},
	} {
		if got := needsPublicIPCertificate(x.desired, x.applied, x.cert, now); got != x.want {
			t.Fatal(x.name, got)
		}
	}
}
