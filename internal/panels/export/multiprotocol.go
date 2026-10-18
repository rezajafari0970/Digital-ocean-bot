package export

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func DecodeVMess(uri string) (map[string]any, error) {
	if !strings.HasPrefix(uri, "vmess://") {
		return nil, errors.New("invalid vmess")
	}
	raw := strings.TrimPrefix(uri, "vmess://")
	b, e := base64.StdEncoding.DecodeString(raw)
	if e != nil {
		b, e = base64.RawStdEncoding.DecodeString(raw)
	}
	if e != nil {
		return nil, e
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil || v == nil {
		return nil, errors.New("invalid vmess")
	}
	host, _ := v["add"].(string)
	id, _ := v["id"].(string)
	if net.ParseIP(host) == nil || id == "" {
		return nil, errors.New("invalid vmess endpoint")
	}
	return v, nil
}
func URIPort(uri string) (int, error) {
	if strings.HasPrefix(uri, "vmess://") {
		v, e := DecodeVMess(uri)
		if e != nil {
			return 0, e
		}
		switch p := v["port"].(type) {
		case string:
			return strconv.Atoi(p)
		case float64:
			if p == float64(int(p)) {
				return int(p), nil
			}
		}
		return 0, errors.New("invalid vmess port")
	}
	u, e := url.Parse(uri)
	if e != nil || u == nil || (u.Scheme != "vless" && u.Scheme != "trojan" && u.Scheme != "ss") {
		return 0, errors.New("invalid profile URI")
	}
	return strconv.Atoi(u.Port())
}
func validAdditionalURI(uri string) bool {
	port, e := URIPort(uri)
	if e != nil || port < 1 || port > 65535 {
		return false
	}
	if strings.HasPrefix(uri, "vmess://") {
		v, e := DecodeVMess(uri)
		if e != nil {
			return false
		}
		network, _ := v["net"].(string)
		tls, _ := v["tls"].(string)
		return (network == "tcp" || network == "ws" || network == "grpc") && (tls == "" || tls == "tls")
	}
	u, e := url.Parse(uri)
	if e != nil || u.User == nil || net.ParseIP(u.Hostname()) == nil {
		return false
	}
	if u.Scheme == "ss" {
		b, e := base64.RawURLEncoding.DecodeString(u.User.Username())
		if e != nil {
			return false
		}
		method, password, ok := strings.Cut(string(b), ":")
		return ok && method == "aes-128-gcm" && len(password) >= 32 && u.RawQuery == ""
	}
	return false
}
