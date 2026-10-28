// Package configdriver composes protocol, transport and security plugins. It has
// no database or network side effects; the control plane owns lifecycle/admission.
package configdriver

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const ManagedPrefix = "dob:profile:"
const CertificateFile = "/opt/dob-trojan/tls/fullchain.pem"
const KeyFile = "/opt/dob-trojan/tls/privkey.pem"

type Spec struct {
	CloudflareOperator    string `json:"cloudflare_operator,omitempty"`
	CloudflareAccount     string `json:"cloudflare_account,omitempty"`
	CloudflareRecords     int    `json:"cloudflare_records,omitempty"`
	CloudflareInterval    int    `json:"cloudflare_interval_seconds,omitempty"`
	CloudflareMinLifetime int    `json:"cloudflare_min_lifetime_seconds,omitempty"`
	CloudflareMaxLifetime int    `json:"cloudflare_max_lifetime_seconds,omitempty"`
	CloudflareMaxActive   int    `json:"cloudflare_max_active,omitempty"`
	Protocol              string `json:"protocol"`
	Network               string `json:"network"`
	HeaderType            string `json:"header_type,omitempty"`
	Security              string `json:"security"`
	Port                  int    `json:"port"`
	Path                  string `json:"path,omitempty"`
	Host                  string `json:"host,omitempty"`
	SNI                   string `json:"sni,omitempty"`
	SNIMode               string `json:"sni_mode,omitempty"`
	SNICandidates         string `json:"sni_candidates,omitempty"`
	Flow                  string `json:"flow,omitempty"`
}
type Client struct {
	ID            string `json:"id"`
	Password      string `json:"password,omitempty"`
	Email         string `json:"email"`
	Class         string `json:"route_class"`
	CreatedAt     int64  `json:"created_at,omitempty"`
	ExpiresAt     int64  `json:"expires_at,omitempty"`
	DeviceLimit   int    `json:"device_limit,omitempty"`
	QuotaBytes    int64  `json:"quota_bytes,omitempty"`
	PolicySet     bool   `json:"policy_set,omitempty"`
	Flow          string `json:"flow,omitempty"`
	Publish       bool   `json:"publish,omitempty"`
	AllowInsecure bool   `json:"allow_insecure,omitempty"`
	FinalMask     string `json:"final_mask,omitempty"`
}
type Credentials struct {
	Clients     []Client         `json:"clients"`
	LastCreated map[string]int64 `json:"last_created,omitempty"`
	PrivateKey  string           `json:"private_key,omitempty"`
	PublicKey   string           `json:"public_key,omitempty"`
	ShortID     string           `json:"short_id,omitempty"`
}
type Capability struct {
	ID                string `json:"id"`
	Protocol          string `json:"protocol"`
	Network           string `json:"network"`
	Security          string `json:"security"`
	Public            bool   `json:"public"`
	Reason            string `json:"reason,omitempty"`
	ClientRequirement string `json:"client_requirement,omitempty"`
}
type Protocol interface {
	Name() string
	Settings(Spec, []Client) map[string]any
	User(Spec, Client) string
}
type Transport interface {
	Name() string
	Server(Spec) map[string]any
	Query(Spec, url.Values)
}
type Security interface {
	Name() string
	Server(Spec, Credentials) map[string]any
	Query(Spec, Credentials, url.Values)
}
type Registry struct {
	protocols    map[string]Protocol
	transports   map[string]Transport
	securities   map[string]Security
	capabilities map[string]Capability
}

func NewRegistry() *Registry {
	return &Registry{map[string]Protocol{}, map[string]Transport{}, map[string]Security{}, map[string]Capability{}}
}
func (r *Registry) RegisterProtocol(p Protocol) error {
	if p == nil || p.Name() == "" || r.protocols[p.Name()] != nil {
		return errors.New("duplicate_or_invalid_protocol")
	}
	r.protocols[p.Name()] = p
	return nil
}
func (r *Registry) RegisterTransport(p Transport) error {
	if p == nil || p.Name() == "" || r.transports[p.Name()] != nil {
		return errors.New("duplicate_or_invalid_transport")
	}
	r.transports[p.Name()] = p
	return nil
}
func (r *Registry) RegisterSecurity(p Security) error {
	if p == nil || p.Name() == "" || r.securities[p.Name()] != nil {
		return errors.New("duplicate_or_invalid_security")
	}
	r.securities[p.Name()] = p
	return nil
}
func key(p, n, s string) string { return p + "/" + n + "/" + s }
func (r *Registry) Register(c Capability) error {
	c.ID = key(c.Protocol, c.Network, c.Security)
	if r.protocols[c.Protocol] == nil || r.transports[c.Network] == nil || r.securities[c.Security] == nil || r.capabilities[c.ID].ID != "" {
		return errors.New("invalid_or_duplicate_capability")
	}
	r.capabilities[c.ID] = c
	return nil
}
func (r *Registry) Catalog() []Capability {
	out := make([]Capability, 0, len(r.capabilities))
	for _, c := range r.capabilities {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)
var serviceName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func ValidatePort(port int) error {
	if port < 1 || port > 65535 || port == 22 || port == 80 || port == 18080 {
		return errors.New("profile_port_reserved")
	}
	return nil
}
func (r *Registry) Validate(s Spec, public bool) error {
	if err := ValidateTCPHeader(s); err != nil {
		return err
	}
	if err := ValidateCloudflare(s); err != nil {
		return err
	}
	c, ok := r.capabilities[key(s.Protocol, s.Network, s.Security)]
	if !ok {
		return errors.New("profile_combination_unsupported")
	}
	if public && !c.Public {
		return errors.New(c.Reason)
	}
	if e := ValidatePort(s.Port); e != nil {
		return e
	}
	if len(s.Host) > 253 || s.Host != "" && !hostname.MatchString(s.Host) {
		return errors.New("profile_host_invalid")
	}
	if s.SNIMode != "" && s.SNIMode != "manual" && s.SNIMode != "auto" {
		return errors.New("profile_sni_mode_invalid")
	}
	if s.Security != "reality" && (s.SNIMode != "" || s.SNICandidates != "") {
		return errors.New("profile_sni_not_applicable")
	}
	if s.SNICandidates != "" {
		names := strings.Split(s.SNICandidates, ",")
		if len(names) > 20 {
			return errors.New("profile_sni_candidates_invalid")
		}
		for _, name := range names {
			if !hostname.MatchString(name) || !strings.Contains(name, ".") || net.ParseIP(name) != nil {
				return errors.New("profile_sni_candidates_invalid")
			}
		}
	}
	if s.Security == "reality" {
		if !(s.SNIMode == "auto" && s.SNI == "") && (!hostname.MatchString(s.SNI) || !strings.Contains(s.SNI, ".") || net.ParseIP(s.SNI) != nil) {
			return errors.New("profile_reality_sni_required")
		}
	} else if s.SNI != "" {
		return errors.New("profile_sni_not_applicable")
	}
	if s.Flow != "" && (s.Protocol != "vless" || s.Network != "tcp" || s.Security != "reality" || s.Flow != "xtls-rprx-vision") {
		return errors.New("profile_flow_unsupported")
	}
	if s.Network == "ws" || s.Network == "xhttp" {
		if !strings.HasPrefix(s.Path, "/") || len(s.Path) > 4096 || strings.ContainsAny(s.Path, "\r\n?#") {
			return errors.New("profile_path_invalid")
		}
	} else if s.Network == "grpc" {
		if !serviceName.MatchString(s.Path) {
			return errors.New("profile_grpc_service_invalid")
		}
	} else if s.Network == "tcp" && s.HeaderType == "http" {
		// Validated by the TCP transport option; other transports stay unchanged.
	} else if s.Path != "" || s.Host != "" {
		return errors.New("profile_transport_options_not_applicable")
	}
	return nil
}
func privateHost(host string) bool {
	ip, e := netip.ParseAddr(host)
	return e == nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}
func NewCredentials(profile, panel string, classes []string, count int) (Credentials, error) {
	c := Credentials{}
	if len(classes) < 1 || len(classes) > 2 || count < 1 || count > 100 {
		return c, errors.New("profile_client_count_invalid")
	}
	seen := map[string]bool{}
	for _, cl := range classes {
		if cl != "DIRECT" && cl != "RESIDENTIAL" || seen[cl] {
			return c, errors.New("profile_class_invalid")
		}
		seen[cl] = true
		for i := 0; i < count; i++ {
			b := make([]byte, 16)
			if _, e := rand.Read(b); e != nil {
				return c, e
			}
			b[6] = (b[6] & 15) | 64
			b[8] = (b[8] & 63) | 128
			id := fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
			pw := make([]byte, 32)
			if _, e := rand.Read(pw); e != nil {
				return c, e
			}
			c.Clients = append(c.Clients, Client{ID: id, Password: base64.RawURLEncoding.EncodeToString(pw), Email: "dob-p-" + profile + "-" + panel + "-" + id, Class: cl})
		}
	}
	k, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		return c, e
	}
	c.PrivateKey = base64.RawURLEncoding.EncodeToString(k.Bytes())
	c.PublicKey = base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
	b := make([]byte, 8)
	if _, e = rand.Read(b); e != nil {
		return c, e
	}
	c.ShortID = hex.EncodeToString(b)
	return c, nil
}
func (r *Registry) Build(s Spec, host, remark string, c Credentials) (map[string]any, error) {
	if net.ParseIP(host) == nil {
		return nil, errors.New("profile_host_invalid")
	}
	if e := r.Validate(s, !privateHost(host)); e != nil {
		return nil, e
	}
	if remark == "" {
		return nil, errors.New("profile_identity_missing")
	}
	ids, emails, pws := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, cl := range c.Clients {
		if cl.ID == "" || cl.Email == "" || ids[cl.ID] || emails[cl.Email] || len(cl.Password) < 32 || pws[cl.Password] {
			return nil, errors.New("profile_credentials_invalid")
		}
		ids[cl.ID] = true
		emails[cl.Email] = true
		pws[cl.Password] = true
	}
	if s.Security == "reality" {
		if s.SNI == "" {
			return nil, errors.New("profile_reality_sni_not_verified")
		}
		k, e := base64.RawURLEncoding.DecodeString(c.PrivateKey)
		if e != nil || len(k) != 32 {
			return nil, errors.New("profile_reality_key_invalid")
		}
		private, e := ecdh.X25519().NewPrivateKey(k)
		if e != nil || base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()) != c.PublicKey {
			return nil, errors.New("profile_reality_key_invalid")
		}
		if b, e := hex.DecodeString(c.ShortID); e != nil || len(b) != 8 {
			return nil, errors.New("profile_short_id_invalid")
		}
	}
	stream := map[string]any{"network": s.Network, "security": s.Security}
	for k, v := range r.transports[s.Network].Server(s) {
		stream[k] = v
	}
	for k, v := range r.securities[s.Security].Server(s, c) {
		stream[k] = v
	}
	return map[string]any{"enable": true, "remark": remark, "listen": "", "port": s.Port, "protocol": s.Protocol, "expiryTime": 0, "total": 0,
		"settings": r.protocols[s.Protocol].Settings(s, c.Clients), "streamSettings": stream,
		"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false, "routeOnly": false}}, nil
}
func (r *Registry) URI(s Spec, host, remark string, c Credentials, cl Client) (string, error) {
	if _, e := r.Build(s, host, "export", c); e != nil {
		return "", e
	}
	owned := false
	for _, x := range c.Clients {
		owned = owned || x == cl
	}
	if !owned {
		return "", errors.New("profile_client_not_owned")
	}
	if s.Protocol == "vmess" && cl.FinalMask == "" {
		tls := ""
		if s.Security == "tls" {
			tls = "tls"
		}
		path := s.Path
		typ := "none"
		if s.Network == "tcp" && s.HeaderType == "http" {
			typ = "http"
			path = tcpHTTPPath(s)
		}
		if s.Network == "grpc" {
			typ = "gun"
		}
		data := map[string]any{"v": "2", "ps": remark, "add": host, "port": strconv.Itoa(s.Port), "id": cl.ID, "aid": "0", "scy": "auto", "net": s.Network, "type": typ, "host": s.Host, "path": path, "tls": tls}
		if s.Security == "tls" && cl.PolicySet {
			data["allowInsecure"] = strconv.FormatBool(cl.AllowInsecure)
		}
		b, _ := json.Marshal(data)
		return "vmess://" + base64.StdEncoding.EncodeToString(b), nil
	}
	q := url.Values{}
	r.transports[s.Network].Query(s, q)
	r.securities[s.Security].Query(s, c, q)
	if s.Protocol == "vless" {
		q.Set("encryption", "none")
		if flow := ClientFlow(s, cl); flow != "" {
			q.Set("flow", flow+"-udp443")
		}
	}
	if s.Security == "tls" && cl.PolicySet {
		q.Set("allowInsecure", strconv.FormatBool(cl.AllowInsecure))
	}
	if cl.FinalMask != "" {
		mask, e := (transportprofile.Config{FinalMask: json.RawMessage(cl.FinalMask)}).ProfileMask()
		if e != nil {
			return "", e
		}
		q.Set("fm", string(mask))
	}
	scheme := s.Protocol
	user := r.protocols[s.Protocol].User(s, cl)
	if s.Protocol == "shadowsocks" {
		if cl.FinalMask != "" {
			return "", errors.New("profile_final_mask_share_unsupported")
		}
		scheme = "ss"
		q = url.Values{}
		if s.HeaderType == "http" {
			q.Set("plugin", shadowsocksHTTPPlugin(s))
		}
	}
	u := url.URL{Scheme: scheme, User: url.User(user), Host: net.JoinHostPort(host, strconv.Itoa(s.Port)), RawQuery: strings.ReplaceAll(q.Encode(), "+", "%20"), Fragment: remark}
	return u.String(), nil
}

type protocolPlugin string

func (p protocolPlugin) Name() string { return string(p) }
func (p protocolPlugin) User(s Spec, c Client) string {
	switch p {
	case "trojan":
		return c.Password
	case "shadowsocks":
		return base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:" + c.Password))
	default:
		return c.ID
	}
}
func (p protocolPlugin) Settings(s Spec, cs []Client) map[string]any {
	clients := []any{}
	for _, c := range cs {
		x := map[string]any{"id": c.ID, "email": c.Email, "enable": true}
		if c.PolicySet {
			x["expiryTime"] = c.ExpiresAt
			x["totalGB"] = c.QuotaBytes
			x["limitHwid"] = c.DeviceLimit
		}
		switch p {
		case "trojan":
			x["password"] = c.Password
		case "shadowsocks":
			x["password"] = c.Password
			x["method"] = "aes-128-gcm"
		case "vless":
			if flow := ClientFlow(s, c); flow != "" {
				x["flow"] = flow
			}
		case "vmess":
			x["security"] = "auto"
		}
		clients = append(clients, x)
	}
	out := map[string]any{"clients": clients}
	switch p {
	case "vless":
		out["decryption"] = "none"
		out["fallbacks"] = []any{}
	case "trojan":
		out["fallbacks"] = []any{}
	case "shadowsocks":
		out["method"] = "aes-128-gcm"
		out["network"] = "tcp"
	}
	return out
}

type transportPlugin string

func (p transportPlugin) Name() string { return string(p) }
func (p transportPlugin) Server(s Spec) map[string]any {
	switch p {
	case "tcp":
		if s.HeaderType == "http" {
			return tcpHTTPSettings(s)
		}
		return nil
	case "ws":
		return map[string]any{"wsSettings": map[string]any{"path": s.Path, "headers": map[string]string{"Host": s.Host}}}
	case "grpc":
		return map[string]any{"grpcSettings": map[string]any{"serviceName": s.Path}}
	case "xhttp":
		return map[string]any{"xhttpSettings": map[string]any{"path": s.Path, "host": s.Host, "mode": "auto"}}
	default:
		return nil
	}
}
func (p transportPlugin) Query(s Spec, q url.Values) {
	q.Set("type", string(p))
	switch p {
	case "tcp":
		if s.HeaderType == "http" {
			q.Set("headerType", "http")
			q.Set("host", s.Host)
			q.Set("path", tcpHTTPPath(s))
		}
	case "ws", "xhttp":
		q.Set("path", s.Path)
		if s.Host != "" {
			q.Set("host", s.Host)
		}
		if p == "xhttp" {
			q.Set("mode", "auto")
		}
	case "grpc":
		q.Set("serviceName", s.Path)
		if s.Host != "" {
			q.Set("authority", s.Host)
		}
		q.Set("mode", "gun")
	}
}

type securityPlugin string

func (p securityPlugin) Name() string { return string(p) }
func (p securityPlugin) Server(s Spec, c Credentials) map[string]any {
	switch p {
	case "tls":
		return map[string]any{"tlsSettings": map[string]any{"minVersion": "1.2", "certificates": []any{map[string]any{"certificateFile": TLSFiles(s, false), "keyFile": TLSFiles(s, true), "oneTimeLoading": false}}}}
	case "reality":
		return map[string]any{"realitySettings": map[string]any{"show": false, "dest": s.SNI + ":443", "serverNames": []string{s.SNI}, "privateKey": c.PrivateKey, "shortIds": []string{c.ShortID}}}
	default:
		return nil
	}
}
func (p securityPlugin) Query(s Spec, c Credentials, q url.Values) {
	q.Set("security", string(p))
	if p == "reality" {
		q.Set("sni", s.SNI)
		q.Set("pbk", c.PublicKey)
		q.Set("sid", c.ShortID)
		q.Set("fp", "chrome")
	}
}
func Builtins() *Registry {
	r := NewRegistry()
	for _, v := range []protocolPlugin{"vless", "vmess", "trojan", "shadowsocks"} {
		if e := r.RegisterProtocol(v); e != nil {
			panic(e)
		}
	}
	for _, v := range []transportPlugin{"tcp", "xhttp", "ws", "grpc"} {
		if e := r.RegisterTransport(v); e != nil {
			panic(e)
		}
	}
	for _, v := range []securityPlugin{"none", "tls", "reality"} {
		if e := r.RegisterSecurity(v); e != nil {
			panic(e)
		}
	}
	add := func(p, n, s string, pub bool, reason string) {
		if e := r.Register(Capability{Protocol: p, Network: n, Security: s, Public: pub, Reason: reason}); e != nil {
			panic(e)
		}
	}
	for _, n := range []string{"tcp", "xhttp", "grpc"} {
		add("vless", n, "reality", true, "")
	}
	for _, n := range []string{"tcp", "ws", "grpc"} {
		add("vless", n, "tls", true, "")
		add("vmess", n, "none", true, "")
		add("vmess", n, "tls", true, "")
		add("trojan", n, "tls", true, "")
	}
	for _, protocol := range []string{"vless", "trojan"} {
		add(protocol, "tcp", "none", true, "")
		cap := r.capabilities[key(protocol, "tcp", "none")]
		cap.ClientRequirement = "plain_tcp_compatible_client"
		r.capabilities[cap.ID] = cap
	}
	add("shadowsocks", "tcp", "none", true, "")
	return r
}
