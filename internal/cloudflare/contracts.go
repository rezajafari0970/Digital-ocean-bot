// Package cloudflare owns CDN resources, independently of panel protocols.
package cloudflare

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

type Auth struct {
	Token string `json:"token,omitempty"`
	Key   string `json:"key,omitempty"`
	Email string `json:"email,omitempty"`
}

func (a Auth) Validate() error {
	if a.Token != "" && a.Key == "" && a.Email == "" && len(a.Token) <= 4096 && !strings.ContainsAny(a.Token, "\r\n ") {
		return nil
	}
	if a.Token == "" && a.Key != "" && len(a.Key) <= 4096 && strings.Contains(a.Email, "@") && !strings.ContainsAny(a.Key+a.Email, "\r\n ") {
		return nil
	}
	return errors.New("cloudflare_credentials_invalid")
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}
type Record struct {
	CreatedOn time.Time `json:"created_on,omitempty"`
	ID        string    `json:"id,omitempty"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Comment   string    `json:"comment"`
	Proxied   bool      `json:"proxied"`
	TTL       int       `json:"ttl"`
}
type Certificate struct {
	ID          string   `json:"id"`
	CSR         string   `json:"csr"`
	Certificate string   `json:"certificate"`
	Hostnames   []string `json:"hostnames"`
	ExpiresOn   string   `json:"expires_on"` // Informational API text; trust the signed certificate expiry only.
}
type DNSDriver interface {
	Zones(context.Context) ([]Zone, error)
	Records(context.Context, string, string) ([]Record, error)
	CreateRecord(context.Context, string, Record) (Record, error)
	DeleteRecord(context.Context, string, string) error
	Setting(context.Context, string, string) (string, error)
	SetSetting(context.Context, string, string, string) error
}
type CertificateDriver interface {
	Certificates(context.Context, string) ([]Certificate, error)
	Issue(context.Context, string, []string) (Certificate, error)
}
type Driver interface {
	DNSDriver
	CertificateDriver
}
type Factory func(context.Context, string) (Driver, error)

var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func ValidID(s string) bool { return uuidPattern.MatchString(s) }
func ValidDomain(s string) bool {
	if len(s) > 190 || !strings.Contains(s, ".") || s != strings.ToLower(s) {
		return false
	}
	if _, e := netip.ParseAddr(s); e == nil {
		return false
	}
	for _, l := range strings.Split(s, ".") {
		if !labelPattern.MatchString(l) {
			return false
		}
	}
	return true
}
func Hostname(zone string) (string, error) {
	if !ValidDomain(zone) {
		return "", errors.New("cloudflare_zone_name_invalid")
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return "assets-stream-delivery-" + hex.EncodeToString(b) + "." + zone, nil
}
func Lifetime(minSeconds, maxSeconds int) (time.Duration, error) {
	if minSeconds < 900 || maxSeconds < minSeconds || maxSeconds > 86400 {
		return 0, errors.New("cloudflare_lifetime_invalid")
	}
	n, e := rand.Int(rand.Reader, big.NewInt(int64(maxSeconds-minSeconds+1)))
	if e != nil {
		return 0, e
	}
	return time.Duration(int64(minSeconds)+n.Int64()) * time.Second, nil
}
func Owns(want, got Record) bool {
	return want.Name == got.Name && want.Type == got.Type && want.Content == got.Content && want.Comment == got.Comment && got.Proxied
}
func CertDirectory(account string) string { return "/opt/dob-cloudflare/" + account + "/current" }
