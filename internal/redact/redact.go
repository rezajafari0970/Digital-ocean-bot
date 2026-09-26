package redact

import (
	"regexp"
	"strings"
)

var sensitiveHeaders = map[string]struct{}{
	"authorization": {}, "cookie": {}, "set-cookie": {}, "proxy-authorization": {}, "x-api-key": {},
}

var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
var assignmentPattern = regexp.MustCompile(`(?i)(password|passwd|token|api[_-]?key|secret)\s*[:=]\s*[^\s]+`)
var privateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)

func Header(name, value string) string {
	if _, ok := sensitiveHeaders[strings.ToLower(strings.TrimSpace(name))]; ok {
		return "[REDACTED]"
	}
	return Text(value)
}

func Text(v string) string {
	v = privateKeyPattern.ReplaceAllString(v, "[PRIVATE KEY REDACTED]")
	v = bearerPattern.ReplaceAllString(v, "Bearer [REDACTED]")
	return assignmentPattern.ReplaceAllString(v, "$1=[REDACTED]")
}
