package adminapi

import (
	"io"
	"net/http"
)

// The human-readable view is opt-in. Subscription URLs retain their exact
// plaintext format and class boundary; no stale configs are cached by the view.
func outputBrowserView(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Configs</title><style>body{margin:12px;font:14px system-ui}#configs{white-space:pre-wrap;overflow-wrap:anywhere;font:14px monospace}#outputStatus{color:#555}</style><body><p id="outputStatus" role="status">Loading current configs…</p><pre id="configs"></pre><script src="/static/output-live.js?v=20261012-status"></script></body></html>`)
}
