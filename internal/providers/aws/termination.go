package aws

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TerminationTarget recognizes only the exact one-instance EC2 mutation emitted
// by this driver. It never consumes/replaces the signed request body.
func TerminationTarget(r *http.Request) string {
	if r.Method != http.MethodPost || r.URL.Scheme != "https" || r.URL.Path != "/" || r.URL.RawQuery != "" || r.GetBody == nil {
		return ""
	}
	host := r.URL.Host
	region := strings.TrimSuffix(strings.TrimPrefix(host, "ec2."), ".amazonaws.com")
	if !regionPattern.MatchString(region) || host != "ec2."+region+".amazonaws.com" {
		return ""
	}
	b, e := r.GetBody()
	if e != nil {
		return ""
	}
	defer b.Close()
	data, e := io.ReadAll(io.LimitReader(b, 4097))
	if e != nil || len(data) > 4096 {
		return ""
	}
	q, e := url.ParseQuery(string(data))
	if e != nil || len(q) != 3 || q.Get("Action") != "TerminateInstances" || q.Get("Version") != "2016-11-15" {
		return ""
	}
	for _, v := range q {
		if len(v) != 1 {
			return ""
		}
	}
	id := q.Get("InstanceId.1")
	if !idPattern.MatchString(id) || !strings.HasPrefix(id, "i-") {
		return ""
	}
	return region + ":" + id
}
