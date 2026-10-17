package trojantcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net"
	"time"
)

const ManagedPrefix = "dob:trojan-tcp:"
const CertificateFile = "/opt/dob-trojan/tls/fullchain.pem"
const KeyFile = "/opt/dob-trojan/tls/privkey.pem"

type Policy struct {
	Enabled     bool     `json:"enabled"`
	Port        int      `json:"port"`
	Direct      bool     `json:"direct_enabled"`
	Residential bool     `json:"residential_enabled"`
	Fleet       bool     `json:"fleet"`
	Panels      []string `json:"panel_ids"`
	Revision    int64    `json:"revision"`
}
type Client struct {
	ID         string `json:"id"`
	Password   string `json:"password"`
	Email      string `json:"email"`
	Enable     bool   `json:"enable"`
	RouteClass string `json:"-"`
}
type Credentials struct{ Direct, Residential Client }
type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, string, []byte) error
}

func NewCredentials(panel string) (Credentials, error) {
	var c Credentials
	for cls, p := range map[string]*Client{"DIRECT": &c.Direct, "RESIDENTIAL": &c.Residential} {
		id, e := sanaei.UUIDv4()
		if e != nil {
			return c, e
		}
		b := make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return c, e
		}
		*p = Client{ID: id, Password: base64.RawURLEncoding.EncodeToString(b), Email: "dob-trojan-" + panel + "-" + cls, Enable: true, RouteClass: cls}
	}
	return c, nil
}
func (c Credentials) Clients(p Policy) []Client {
	out := []Client{}
	if p.Direct {
		x := c.Direct
		x.RouteClass = "DIRECT"
		out = append(out, x)
	}
	if p.Residential {
		x := c.Residential
		x.RouteClass = "RESIDENTIAL"
		out = append(out, x)
	}
	return out
}
func ValidatePort(port, panelPort int, reserved, occupied []int) error {
	if port < 1024 || port > 65535 || port == panelPort || port == 22 || port == 80 || port == 443 || port == 18080 {
		return errors.New("trojan_port_reserved")
	}
	for _, ps := range [][]int{reserved, occupied} {
		for _, p := range ps {
			if port == p {
				return errors.New("trojan_port_occupied")
			}
		}
	}
	return nil
}
func Build(p Policy, host string, clients []Client) (map[string]any, error) {
	if net.ParseIP(host) == nil || ValidatePort(p.Port, 0, nil, nil) != nil || len(clients) == 0 {
		return nil, errors.New("trojan_invalid_config")
	}
	ids, emails, passwords := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range clients {
		if c.ID == "" || c.Email == "" || len(c.Password) < 32 || !c.Enable || ids[c.ID] || emails[c.Email] || passwords[c.Password] {
			return nil, errors.New("trojan_invalid_identity")
		}
		ids[c.ID] = true
		emails[c.Email] = true
		passwords[c.Password] = true
	}
	return map[string]any{
		"enable": true, "remark": fmt.Sprintf("%s%05d", ManagedPrefix, p.Port), "listen": "", "port": p.Port, "protocol": "trojan", "expiryTime": 0, "total": 0,
		"settings": map[string]any{"clients": clients, "fallbacks": []any{}},
		"streamSettings": map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{
			"minVersion": "1.2", "certificates": []any{map[string]any{"certificateFile": CertificateFile, "keyFile": KeyFile, "oneTimeLoading": false}},
		}},
		"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false, "routeOnly": false},
	}, nil
}
func decode(raw json.RawMessage) (map[string]any, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		raw = []byte(s)
	}
	var out map[string]any
	e := json.Unmarshal(raw, &out)
	if out == nil && e == nil {
		e = errors.New("invalid object")
	}
	return out, e
}
func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type TLSProof struct {
	NotAfter time.Time
	SHA256   string
}
