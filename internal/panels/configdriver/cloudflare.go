package configdriver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflareschedule"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var cfID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func ValidateCloudflare(s Spec) error {
	if s.CloudflareAccount == "" {
		if s.CloudflareOperator != "" {
			return errors.New("profile_cloudflare_account_required")
		}
		if s.CloudflareRecords != 0 || s.CloudflareInterval != 0 || s.CloudflareMinLifetime != 0 || s.CloudflareMaxLifetime != 0 || s.CloudflareMaxActive != 0 {
			return errors.New("profile_cloudflare_account_required")
		}
		return nil
	}
	if !validOperator(s.CloudflareOperator) {
		return errors.New("profile_cloudflare_operator_invalid")
	}
	if !cfID.MatchString(s.CloudflareAccount) {
		return errors.New("profile_cloudflare_account_invalid")
	}
	// WS is the first supported CDN transport. Raw TCP/REALITY never pass this gate.
	if s.Security != "tls" || s.Network != "ws" {
		return errors.New("profile_cloudflare_requires_ws_tls")
	}
	if s.Host != "" || s.SNI != "" {
		return errors.New("profile_cloudflare_manages_host")
	}
	if !CloudflarePort(s.Port) {
		return errors.New("profile_cloudflare_port_invalid")
	}
	return (cloudflareschedule.Schedule{Records: s.CloudflareRecords, Interval: s.CloudflareInterval, MinLifetime: s.CloudflareMinLifetime, MaxLifetime: s.CloudflareMaxLifetime, MaxActive: s.CloudflareMaxActive}).Validate()
}

func CloudflarePort(p int) bool {
	switch p {
	case 443, 2053, 2083, 2087, 2096, 8443:
		return true
	}
	return false
}
func TLSFiles(s Spec, key bool) string {
	if s.CloudflareAccount != "" && cfID.MatchString(s.CloudflareAccount) {
		name := "fullchain.pem"
		if key {
			name = "privkey.pem"
		}
		return "/opt/dob-cloudflare/" + s.CloudflareAccount + "/current/" + name
	}
	if key {
		return KeyFile
	}
	return CertificateFile
}
func validCDNHost(h string) bool {
	if len(h) > 253 || h != strings.ToLower(h) || !strings.Contains(h, ".") || net.ParseIP(h) != nil {
		return false
	}
	for _, part := range strings.Split(h, ".") {
		if len(part) < 1 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func (r *Registry) CDNURI(s Spec, origin, address, domain, remark string, c Credentials, cl Client) (string, error) {
	if s.CloudflareAccount == "" || !validCDNHost(domain) || (address != domain && net.ParseIP(address) == nil) || cl.AllowInsecure {
		return "", errors.New("profile_cloudflare_endpoint_invalid")
	}
	raw, e := r.URI(s, origin, remark, c, cl)
	if e != nil {
		return "", e
	}
	if s.Protocol == "vmess" && !strings.Contains(raw, "?") {
		b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "vmess://"))
		if e != nil {
			return "", e
		}
		var v map[string]any
		if e = json.Unmarshal(b, &v); e != nil {
			return "", e
		}
		v["add"] = address
		v["host"] = domain
		v["sni"] = domain
		v["allowInsecure"] = "false"
		b, e = json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b), e
	}
	u, e := url.Parse(raw)
	if e != nil {
		return "", e
	}
	u.Host = net.JoinHostPort(address, strconv.Itoa(s.Port))
	q := u.Query()
	q.Set("host", domain)
	q.Set("sni", domain)
	q.Set("allowInsecure", "false")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (r *Registry) CDNOutbound(s Spec, origin, address, domain string, c Credentials, cl Client) (map[string]any, error) {
	if s.CloudflareAccount == "" || !validCDNHost(domain) || (address != domain && net.ParseIP(address) == nil) || cl.AllowInsecure {
		return nil, errors.New("profile_cloudflare_endpoint_invalid")
	}
	out, e := r.Outbound(s, origin, c, cl)
	if e != nil {
		return nil, e
	}
	stream := out["streamSettings"].(map[string]any)
	stream["tlsSettings"] = map[string]any{"serverName": domain, "allowInsecure": false}
	stream["wsSettings"] = map[string]any{"path": s.Path, "headers": map[string]string{"Host": domain}}
	settings := out["settings"].(map[string]any)
	key := "vnext"
	if s.Protocol == "trojan" {
		key = "servers"
	}
	settings[key].([]any)[0].(map[string]any)["address"] = address
	return out, nil
}

func validOperator(s string) bool {
	switch s {
	case "", "mci", "irancell", "rightel", "mokhaberat", "shatel", "other":
		return true
	}
	return false
}
