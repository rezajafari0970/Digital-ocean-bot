package geoctx

import "net/http"

func ApplyRequest(req *http.Request, locale string) {
	if req != nil && locale != "" {
		req.Header.Set("Accept-Language", locale+","+locale[:2]+";q=0.9")
	}
}
