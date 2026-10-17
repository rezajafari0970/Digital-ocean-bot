package export

import (
	"net/url"
	"strings"
)

// WithRemark changes only the URI fragment. It deliberately preserves every
// byte of the connection string, including escapes, query order and duplicates.
func WithRemark(uri, remark string) string {
	head, _, _ := strings.Cut(uri, "#")
	return head + "#" + strings.ReplaceAll(url.QueryEscape(remark), "+", "%20")
}
