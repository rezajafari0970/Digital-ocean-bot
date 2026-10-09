package residentialsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"net"
	"os"
	"strings"
)

const RelayProbeURL = "https://browserleaks.com/ip"
const RelayMinimumHealthy = 3

type relayTransport struct{ SecretRef, Certificate string }

func RelayEnabled(panel string) bool { return relayScopeEnabled("DOB_UPCLOUD_RELAY_PANELS", panel) }
func RelayExpandedEnabled(panel string) bool {
	return relayScopeEnabled("DOB_UPCLOUD_RELAY_EXPANDED_PANELS", panel)
}
func relayScopeEnabled(key, panel string) bool {
	scope := strings.TrimSpace(os.Getenv(key))
	if scope == "all" {
		return true
	}
	if scope == "" || scope == "none" {
		return false
	}
	selected := false
	for _, id := range strings.Split(scope, ",") {
		id = strings.TrimSpace(id)
		if !residentialperf.UUID.MatchString(id) {
			return false
		}
		selected = selected || strings.EqualFold(id, panel)
	}
	return selected
}
func relayConfigured() bool {
	scope := strings.TrimSpace(os.Getenv("DOB_UPCLOUD_RELAY_PANELS"))
	if scope == "all" {
		return true
	}
	for _, id := range strings.Split(scope, ",") {
		if !residentialperf.UUID.MatchString(strings.TrimSpace(id)) {
			return false
		}
	}
	return scope != ""
}
func categoryDigest(p routePolicy) string {
	raw, _ := json.Marshal(struct {
		Domains            []string
		Strict, Paths, Ads bool
	}{p.residentialDomains(), p.StrictAllowlist, !p.LegacyClientPaths, p.AdsOnly})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func managedSOCKS(donor, host, ref string, port int, c relayCredential) (rp, error) {
	ip := net.ParseIP(host)
	if !residentialperf.UUID.MatchString(donor) || ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || !providers.UpCloudTrialProxyPortAllowed(port) || ref == "" || c.User == "" || len(c.Password) < 32 || c.Certificate == "" {
		return rp{}, errors.New("invalid managed SOCKS transport")
	}
	data, _ := json.Marshal(struct {
		Host                        string
		Port                        int
		User, Password, Certificate string
	}{host, port, c.User, c.Password, c.Certificate})
	h := sha256.Sum256(data)
	digest := hex.EncodeToString(h[:])
	return rp{ID: donor, Type: "socks5", Host: host, Port: port, User: c.User, Password: c.Password,
		Tag: "residential-ads-relay-" + donor + "-" + digest[:12], Relay: &relayTransport{ref, c.Certificate},
		TransportHash: digest}, nil
}
func proxyHasUDP(p rp) bool { return p.Type == "socks5" }
func proxyOutbound(p rp) (map[string]any, error) {
	protocol := "http"
	if p.Type == "socks5" {
		protocol = "socks"
	} else if p.Type != "http" && p.Type != "https" {
		return nil, errors.New("unsupported residential protocol")
	}
	server := map[string]any{"address": p.Host, "port": p.Port}
	if p.User != "" || p.Password != "" {
		server["users"] = []any{map[string]any{"user": p.User, "pass": p.Password}}
	}
	o := map[string]any{"tag": p.Tag, "protocol": protocol, "settings": map[string]any{"servers": []any{server}}}
	if p.Type == "https" {
		o["streamSettings"] = map[string]any{"security": "tls", "tlsSettings": map[string]any{"serverName": p.Host}}
	}
	if p.Relay != nil {
		o["streamSettings"] = map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{
			"serverName": "dob-relay.internal", "disableSystemRoot": true,
			"certificates": []any{map[string]any{"usage": "verify", "certificate": strings.Split(strings.TrimSpace(p.Relay.Certificate), "\n")}}},
			"sockopt": map[string]any{"domainStrategy": "ForceIPv4"}}
		o["mux"] = map[string]any{"enabled": true, "concurrency": -1, "xudpConcurrency": 8, "xudpProxyUDP443": "allow"}
	}
	return o, nil
}

// Relay admission also binds this generated transport to its durable donor record.
func ValidateRelayOutbound(o map[string]any) bool {
	raw, _ := json.Marshal(o)
	var v struct {
		Tag, Protocol string
		Settings      struct {
			Servers []struct {
				Address string
				Port    int
				Users   []struct{ User, Pass string }
			}
		}
		Mux struct {
			Enabled                      bool
			Concurrency, XudpConcurrency int
			XudpProxyUDP443              string
		}
		StreamSettings struct {
			Network, Security string
			TLSSettings       struct {
				ServerName        string
				DisableSystemRoot bool
				Certificates      []struct {
					Usage       string
					Certificate []string
				}
			}
		}
	}
	if json.Unmarshal(raw, &v) != nil || v.Protocol != "socks" || !strings.HasPrefix(v.Tag, "residential-ads-relay-") || len(v.Settings.Servers) != 1 {
		return false
	}
	s := v.Settings.Servers[0]
	t := v.StreamSettings
	if !(providers.UpCloudTrialProxyPortAllowed(s.Port) && net.ParseIP(s.Address) != nil && len(s.Users) == 1 && s.Users[0].User != "" && len(s.Users[0].Pass) >= 32 && t.Security == "tls" && t.Network == "tcp" && t.TLSSettings.ServerName == "dob-relay.internal" && t.TLSSettings.DisableSystemRoot && len(t.TLSSettings.Certificates) == 1 && t.TLSSettings.Certificates[0].Usage == "verify" && v.Mux.Enabled && v.Mux.Concurrency == -1 && v.Mux.XudpConcurrency == 8 && v.Mux.XudpProxyUDP443 == "allow") {
		return false
	}
	prefix := strings.TrimPrefix(v.Tag, "residential-ads-relay-")
	if len(prefix) < 36 {
		return false
	}
	credential := relayCredential{User: s.Users[0].User, Password: s.Users[0].Pass, Certificate: strings.Join(t.TLSSettings.Certificates[0].Certificate, "\n") + "\n"}
	proxy, e := managedSOCKS(prefix[:36], s.Address, "validation", s.Port, credential)
	if e != nil {
		return false
	}
	if v.Tag != proxy.Tag {
		suffix := strings.TrimPrefix(v.Tag, proxy.Tag+"-")
		if len(suffix) != 16 {
			return false
		}
		if _, e = hex.DecodeString(suffix); e != nil {
			return false
		}
	}
	want, e := proxyOutbound(proxy)
	if e != nil {
		return false
	}
	want["tag"] = v.Tag
	var have map[string]any
	if json.Unmarshal(raw, &have) != nil {
		return false
	}
	hs, ok := have["streamSettings"].(map[string]any)
	if !ok {
		return false
	}
	sock, ok := hs["sockopt"].(map[string]any)
	if !ok || sock["domainStrategy"] != "ForceIPv4" {
		return false
	}
	ws := want["streamSettings"].(map[string]any)["sockopt"].(map[string]any)
	for k, v := range sock {
		switch k {
		case "domainStrategy", "tcpFastOpen", "tcpKeepAliveIdle", "tcpKeepAliveInterval", "tcpUserTimeout":
			ws[k] = v
		default:
			return false
		}
	}
	a, _ := json.Marshal(have)
	b, _ := json.Marshal(want)
	return string(a) == string(b)
}
