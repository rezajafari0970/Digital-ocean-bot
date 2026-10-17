package export

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// TrojanTCP exports only a verified TLS identity. It never exports allowInsecure,
// VLESS flow, Reality keys, XHTTP options or a TLS hostname unrelated to this IP.
type TrojanTCP struct {
	Host             string
	Port             int
	Password, Remark string
}

func TrojanTCPURI(c TrojanTCP) (string, error) {
	ip := net.ParseIP(c.Host)
	if ip == nil || c.Port < 1 || c.Port > 65535 || len(c.Password) < 32 || strings.ContainsAny(c.Password, "\r\n\x00") {
		return "", errors.New("invalid Trojan TLS export")
	}
	q := url.Values{"security": {"tls"}, "type": {"tcp"}}
	u := url.URL{Scheme: "trojan", User: url.User(c.Password), Host: net.JoinHostPort(ip.String(), strconv.Itoa(c.Port)), RawQuery: q.Encode(), Fragment: c.Remark}
	return u.String(), nil
}
