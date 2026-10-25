package configdriver

import (
	"errors"
	"net/url"
	"strings"
)

// TCP HTTP headers are an explicit transport option. They do not enable TLS,
// change routing, or relax protocol/client security requirements.
func validateTCPHeader(s Spec) error {
	if s.HeaderType == "" {
		return nil
	}
	if s.Network != "tcp" {
		return errors.New("profile_tcp_header_not_applicable")
	}
	if s.HeaderType != "none" && s.HeaderType != "http" {
		return errors.New("profile_tcp_header_invalid")
	}
	if s.HeaderType != "http" {
		return nil
	}
	if s.Security == "reality" || s.Flow != "" {
		return errors.New("profile_tcp_header_security_unsupported")
	}
	if s.Host == "" {
		return errors.New("profile_http_host_required")
	}
	path := tcpHTTPPath(s)
	if len(path) > 4096 || !strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\r\n?#;=") {
		return errors.New("profile_path_invalid")
	}
	for _, c := range path {
		if c <= 32 || c == 127 {
			return errors.New("profile_path_invalid")
		}
	}
	if _, err := url.ParseRequestURI(path); err != nil {
		return errors.New("profile_path_invalid")
	}
	return nil
}
func tcpHTTPPath(s Spec) string {
	if s.Path == "" {
		return "/"
	}
	return s.Path
}
func tcpHTTPSettings(s Spec) map[string]any {
	return map[string]any{"tcpSettings": map[string]any{
		"header": map[string]any{
			"type": "http",
			"request": map[string]any{
				"version": "1.1", "method": "GET", "path": []string{tcpHTTPPath(s)},
				"headers": map[string]any{"Host": []string{s.Host}},
			},
			"response": map[string]any{},
		},
	}}
}

// v2rayNG's SIP002 parser reads HTTP transport from the plugin parameter;
// generic VLESS headerType/host query fields are ignored for Shadowsocks.
func shadowsocksHTTPPlugin(s Spec) string {
	return "obfs-local;obfs=http;obfs-host=" + s.Host + ";path=" + tcpHTTPPath(s)
}
