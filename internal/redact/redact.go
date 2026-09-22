package redact

import (
	"regexp"
	"strings"
)

var sensitiveHeaders = map[string]struct{}{
	"authorization": {}, "cookie": {}, "set-cookie": {}, "proxy-authorization": {}, "x-api-key": {},
}

var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)

func Header(name, value string) string {
	if _, ok := sensitiveHeaders[strings.ToLower(strings.TrimSpace(name))]; ok {
		return "[REDACTED]"
	}
	return Text(value)
}

func Text(v string) string { return bearerPattern.ReplaceAllString(v, "Bearer [REDACTED]") }
