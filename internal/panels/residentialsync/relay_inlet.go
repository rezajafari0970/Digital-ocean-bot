package residentialsync

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

const relayInletPrefix = "dob-route-relay-in-"

type relayCredential struct{ User, Password, Certificate, Key string }
type relayInlet struct {
	Panel, Host, SecretRef string
	Port                   int
	Credential             relayCredential
	Sources                []string
}

func newRelayCredential() (relayCredential, error) {
	var c relayCredential
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return c, e
	}
	c.Password = base64.RawURLEncoding.EncodeToString(b)
	if _, e := rand.Read(b[:12]); e != nil {
		return c, e
	}
	c.User = "dob-" + base64.RawURLEncoding.EncodeToString(b[:12])
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return c, e
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		return c, e
	}
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "dob-relay.internal"}, DNSNames: []string{"dob-relay.internal"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(7 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		return c, e
	}
	kb, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return c, e
	}
	c.Certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	c.Key = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}))
	return c, nil
}
func relayCredentialExpiry(c relayCredential) time.Time {
	cert, _ := pem.Decode([]byte(c.Certificate))
	if cert == nil {
		return time.Time{}
	}
	v, e := x509.ParseCertificate(cert.Bytes)
	if e != nil {
		return time.Time{}
	}
	return v.NotAfter
}
func relayCredentialValid(c relayCredential) bool {
	cert, _ := pem.Decode([]byte(c.Certificate))
	if cert == nil {
		return false
	}
	v, e := x509.ParseCertificate(cert.Bytes)
	return e == nil && time.Now().Add(24*time.Hour).Before(v.NotAfter) && len(c.Password) >= 32 && c.User != "" && c.Key != ""
}
func chooseRelayPort(raws []json.RawMessage, current map[string]any, tag string) (int, error) {
	used := map[int]bool{}
	inspect := func(m map[string]any) error {
		if m["tag"] == tag {
			return nil
		}
		if m["port"] == nil {
			return nil
		}
		s := strings.TrimSpace(strings.Trim(fmtSprint(m["port"]), "\""))
		for _, part := range strings.Split(s, ",") {
			edges := strings.Split(part, "-")
			low, e := strconv.Atoi(strings.TrimSpace(edges[0]))
			if e != nil {
				return errors.New("unrecognized inbound port")
			}
			high := low
			if len(edges) == 2 {
				high, e = strconv.Atoi(strings.TrimSpace(edges[1]))
				if e != nil {
					return e
				}
			} else if len(edges) > 2 {
				return errors.New("unrecognized inbound port range")
			}
			for _, candidate := range []int{8080, 80, 443} {
				if low <= candidate && candidate <= high {
					used[candidate] = true
				}
			}
		}
		return nil
	}
	for _, raw := range raws {
		m, e := decodeObject(raw)
		if e != nil {
			return 0, e
		}
		if e = inspect(m); e != nil {
			return 0, e
		}
	}
	if ins, ok := current["inbounds"].([]any); ok {
		for _, v := range ins {
			m, ok := v.(map[string]any)
			if !ok {
				return 0, errors.New("invalid template inbound")
			}
			if e := inspect(m); e != nil {
				return 0, e
			}
		}
	}
	for _, port := range []int{8080, 80, 443} {
		if !used[port] {
			return port, nil
		}
	}
	return 0, errors.New("no free trial-permitted SOCKS port")
}
func fmtSprint(v any) string { b, _ := json.Marshal(v); return string(b) }

func configureRelayInlet(next, current map[string]any, p routePolicy, destination string, rules *[]any) error {
	var kept []any
	if ins, ok := next["inbounds"].([]any); ok {
		for _, v := range ins {
			m, ok := v.(map[string]any)
			if !ok {
				return errors.New("invalid template inbound")
			}
			tag, _ := m["tag"].(string)
			if strings.HasPrefix(tag, relayInletPrefix) {
				continue
			}
			kept = append(kept, v)
		}
	}
	if p.RelayInlet == nil {
		if _, ok := next["inbounds"]; ok {
			next["inbounds"] = kept
		}
		return nil
	}
	in := p.RelayInlet
	if p.RelayMode || !p.StrictAllowlist || !p.AdsOnly || !p.Harden || p.LegacyClientPaths {
		return errors.New("relay inlet requires hardened category policy")
	}
	if len(in.Sources) == 0 || !relayCredentialValid(in.Credential) {
		return errors.New("relay inlet credential or sources unavailable")
	}
	for _, source := range in.Sources {
		ip, network, e := net.ParseCIDR(source)
		if e != nil || ip.To4() == nil {
			return errors.New("invalid relay source")
		}
		ones, _ := network.Mask.Size()
		if ones != 32 {
			return errors.New("relay source must be one IPv4")
		}
	}
	tag := relayInletPrefix + in.Panel
	kept = append(kept, map[string]any{"tag": tag, "listen": in.Host, "port": in.Port, "protocol": "socks",
		"settings": map[string]any{"auth": "password", "accounts": []any{map[string]any{"user": in.Credential.User, "pass": in.Credential.Password}}, "udp": false},
		"streamSettings": map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"minVersion": "1.2", "certificates": []any{map[string]any{
			"certificate": strings.Split(strings.TrimSpace(in.Credential.Certificate), "\n"), "key": strings.Split(strings.TrimSpace(in.Credential.Key), "\n")}}}},
		"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true}})
	next["inbounds"] = kept
	sources := append([]string{}, in.Sources...)
	sort.Strings(sources)
	if p.Residential && !p.SniffingBlocked && len(p.Proxies) > 0 {
		*rules = append(*rules, map[string]any{"type": "field", "ruleTag": "dob-route-relay-ads", "inboundTag": []string{tag},
			"source": sources, "user": []string{in.Credential.User}, "domain": p.residentialDomains(), "network": "tcp,udp", "outboundTag": destination})
	}
	*rules = append(*rules, map[string]any{"type": "field", "ruleTag": "dob-route-relay-deny", "inboundTag": []string{tag}, "network": "tcp,udp", "outboundTag": blockedTag})
	return nil
}
