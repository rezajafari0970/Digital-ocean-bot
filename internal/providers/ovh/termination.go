package ovh

import (
	"net/http"
	"strings"
)

// TerminationTarget accepts only an exact project-scoped instance DELETE.
func TerminationTarget(r *http.Request) (id, external string) {
	if r.Method != http.MethodDelete || r.URL.Scheme != "https" || r.URL.RawQuery != "" {
		return "", ""
	}
	endpoint := ""
	for name, base := range endpoints {
		if strings.HasPrefix(r.URL.String(), base+"/") {
			endpoint = name
			break
		}
	}
	if endpoint == "" {
		return "", ""
	}
	p := strings.Split(r.URL.Path, "/")
	if len(p) != 7 || p[1] != "1.0" || p[2] != "cloud" || p[3] != "project" || !projectPattern.MatchString(p[4]) || p[5] != "instance" || !uuidPattern.MatchString(p[6]) {
		return "", ""
	}
	return p[6], endpoint + ":" + p[4]
}
