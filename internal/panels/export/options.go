package export

import (
	"net/url"
	"strings"
)

// OutputOptions control publication only. Stored native contracts and complete
// snapshot URIs remain unchanged.
type OutputOptions struct {
	IncludeXHTTPExtra bool `json:"include_xhttp_extra"`
	IncludeFinalMask  bool `json:"include_finalmask"`
}

// ApplyOutputOptions preserves retained components byte-for-byte. In
// particular, it must not form-decode '+' or re-encode existing URI values.
func ApplyOutputOptions(uri string, options OutputOptions) (string, error) {
	u, err := url.Parse(uri)
	if err == nil && u.Scheme == "trojan" && u.User != nil && u.Host != "" {
		q, e := url.ParseQuery(u.RawQuery)
		if e != nil || q.Get("security") != "tls" || q.Get("type") != "tcp" {
			return "", ErrInvalidVLESS
		}
		for k := range q {
			if k != "security" && k != "type" {
				return "", ErrInvalidVLESS
			}
		}
		return uri, nil
	}
	if err != nil || u.Scheme != "vless" || u.User == nil || u.Host == "" {
		return "", ErrInvalidVLESS
	}
	head, fragment, hasFragment := strings.Cut(uri, "#")
	base, query, hasQuery := strings.Cut(head, "?")
	if !hasQuery || query == "" {
		return uri, nil
	}
	kept := make([]string, 0)
	for _, part := range strings.Split(query, "&") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			return "", ErrInvalidVLESS
		}
		name, e := url.PathUnescape(key)
		if e != nil {
			return "", ErrInvalidVLESS
		}
		if _, e = url.PathUnescape(value); e != nil {
			return "", ErrInvalidVLESS
		}
		switch name {
		case "extra":
			if !options.IncludeXHTTPExtra {
				continue
			}
		case "fm", "finalmask":
			if !options.IncludeFinalMask {
				continue
			}
		case "browserDialer", "browserDialerMode", "browser_dialer":
			// Browser activation is not a server-to-client URI feature.
			continue
		}
		kept = append(kept, part)
	}
	out := base + "?" + strings.Join(kept, "&")
	if hasFragment {
		out += "#" + fragment
	}
	return out, nil
}
