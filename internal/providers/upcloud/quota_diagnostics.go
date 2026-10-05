package upcloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Names and shapes are application-owned vocabulary. Unknown keys and values
// may contain secrets; they must never be interpolated into diagnostics.
func quotaDiagnosticName(key string) string {
	if known, _ := resourceQuotaNullability(key); known {
		return strings.ToUpper(key)
	}
	return "OTHER_FIELD"
}

var quotaNumericSyntax = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func quotaValueShape(raw []byte) string {
	b := bytes.TrimSpace(raw)
	if len(b) == 0 {
		return "EMPTY"
	}
	if bytes.Equal(b, []byte("null")) {
		return "NULL"
	}
	switch b[0] {
	case '{':
		return "OBJECT"
	case '[':
		return "ARRAY"
	case 't', 'f':
		return "BOOLEAN"
	}
	suffix := "NUMBER"
	s := string(b)
	if b[0] == '"' {
		suffix = "STRING"
		if json.Unmarshal(b, &s) != nil {
			return "MALFORMED_STRING"
		}
	}
	// Classification only, never conversion or an alternate acceptance path.
	if !quotaNumericSyntax.MatchString(s) {
		return "NON_NUMERIC_" + suffix
	}
	if strings.ContainsAny(s, "eE") {
		return "EXPONENT_" + suffix
	}
	if strings.Contains(s, ".") {
		return "DECIMAL_" + suffix
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v > 1<<50 {
		return "OUT_OF_RANGE_" + suffix
	}
	if v < 0 {
		return "NEGATIVE_" + suffix
	}
	return "UNSUPPORTED_" + suffix
}

func quotaIssue(key string, raw []byte) string {
	return "INVALID_QUOTA_" + quotaDiagnosticName(key) + "_" + quotaValueShape(raw)
}

// QuotaIssues returns only generated, bounded field/type diagnostics. It never
// returns the raw quota values or arbitrary response field names.
func QuotaIssues(err error) []string {
	var r *responseError
	if !errors.As(err, &r) {
		return nil
	}
	var out []string
	for _, issue := range r.QuotaIssues {
		if validQuotaIssue(issue) {
			out = append(out, issue)
			if len(out) == 64 {
				break
			}
		}
	}
	return out
}

// Revalidate at the export boundary even though the decoder constructs codes.
func validQuotaIssue(code string) bool {
	if !safeDiagnostic.MatchString(code) || !strings.HasPrefix(code, "INVALID_QUOTA_") {
		return false
	}
	rest := strings.TrimPrefix(code, "INVALID_QUOTA_")
	for _, shape := range []string{
		"EMPTY", "NULL", "OBJECT", "ARRAY", "BOOLEAN", "MALFORMED_STRING",
		"NON_NUMERIC_NUMBER", "NON_NUMERIC_STRING", "EXPONENT_NUMBER", "EXPONENT_STRING",
		"DECIMAL_NUMBER", "DECIMAL_STRING", "OUT_OF_RANGE_NUMBER", "OUT_OF_RANGE_STRING",
		"NEGATIVE_NUMBER", "NEGATIVE_STRING", "UNSUPPORTED_NUMBER", "UNSUPPORTED_STRING",
	} {
		field, ok := strings.CutSuffix(rest, "_"+shape)
		if ok && (field == "OTHER_FIELD" || quotaDiagnosticName(strings.ToLower(field)) == field) {
			return true
		}
	}
	return false
}
