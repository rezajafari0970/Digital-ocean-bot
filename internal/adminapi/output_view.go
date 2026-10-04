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
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Live configs</title><style>
body{margin:0;padding:24px;background:#0b1020;color:#e5eaff;font:16px system-ui}h1{font-size:22px}#status{padding:16px;background:#172038;border-radius:12px}pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px}button{padding:12px;margin:12px 0;background:#4e76ff;border:0;border-radius:8px;color:white}
</style><h1>Live configs</h1><p id="status" role="status">Checking fresh configs…</p><button id="copy" disabled>Copy configs</button><pre id="configs"></pre><script src="/admin/output-live.js?v=20261004"></script></html>`)
}
