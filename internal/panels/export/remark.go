package export

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

// WithRemark changes only the URI fragment. It deliberately preserves every
// byte of the connection string, including escapes, query order and duplicates.
func WithRemark(uri, remark string) string {
	if strings.HasPrefix(uri, "vmess://") && !strings.Contains(uri, "?") {
		if v, e := DecodeVMess(uri); e == nil {
			v["ps"] = remark
			b, _ := json.Marshal(v)
			return "vmess://" + base64.StdEncoding.EncodeToString(b)
		}
		return uri
	}
	head, _, _ := strings.Cut(uri, "#")
	return head + "#" + strings.ReplaceAll(url.QueryEscape(remark), "+", "%20")
}
