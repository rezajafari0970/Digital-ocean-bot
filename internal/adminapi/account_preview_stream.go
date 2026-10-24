package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

const previewStreamType = "application/x-ndjson"

type previewProgressKey struct{}

func previewStage(ctx context.Context, stage string) {
	if p, ok := ctx.Value(previewProgressKey{}).(*atomic.Value); ok {
		p.Store(stage)
	}
}

type previewResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (w *previewResponse) Header() http.Header { return w.header }
func (w *previewResponse) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}
func (w *previewResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(b) > 8<<20 {
		w.overflow = true
		return 0, errors.New("preview response too large")
	}
	return w.body.Write(b)
}

// Only read-only account discovery opts into progress streaming. Credentials stay
// in this request; there is no background job, persistent token or automatic retry.
func accountPreviewResponse(w http.ResponseWriter, r *http.Request, next http.HandlerFunc, budget, heartbeat time.Duration) {
	if r.Header.Get("Accept") != previewStreamType {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		next(w, r.WithContext(ctx))
		return
	}
	// Consume and bound the body before flushing HTTP/1 response headers.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	defer zeroBytes(body)
	ctx, cancel := context.WithTimeout(r.Context(), budget)
	defer cancel()
	stage := &atomic.Value{}
	stage.Store("connection")
	ctx = context.WithValue(ctx, previewProgressKey{}, stage)
	r = r.Clone(ctx)
	r.Body = io.NopCloser(bytes.NewReader(body))
	controller := http.NewResponseController(w)
	if err = controller.SetWriteDeadline(time.Now().Add(budget + 10*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeJSON(w, 500, map[string]string{"error": "preview_stream_unavailable"})
		return
	}
	w.Header().Set("Content-Type", previewStreamType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	started := time.Now()
	encode := json.NewEncoder(w)
	progress := func() bool {
		if encode.Encode(map[string]any{"type": "progress", "stage": stage.Load(), "elapsed_seconds": int(time.Since(started).Seconds())}) != nil {
			cancel()
			return false
		}
		if controller.Flush() != nil {
			cancel()
			return false
		}
		return true
	}
	if !progress() {
		return
	}
	stopped, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				if !progress() {
					return
				}
			}
		}
	}()
	// The handler and its response buffer remain on this goroutine. Join the only
	// writer goroutine before sending the final envelope or returning.
	joined := false
	join := func() {
		if !joined {
			close(stopped)
			<-done
			joined = true
		}
	}
	defer join()
	result := &previewResponse{header: make(http.Header)}
	next(result, r)
	join()
	if r.Context().Err() == context.Canceled {
		return
	}
	status, raw := result.status, result.body.Bytes()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = 504
		raw = []byte(`{"error":"provider_preview_timeout","detail":"Provider validation timed out. Your entries are preserved. Try fewer regions or check the selected connection, then validate again."}`)
	} else if result.overflow || !json.Valid(raw) || status < 200 || status > 599 {
		status = 502
		raw = []byte(`{"error":"provider_preview_invalid_response","detail":"Provider validation returned an incomplete response. Validate again before saving."}`)
	}
	_ = encode.Encode(map[string]any{"type": "result", "status": status, "body": json.RawMessage(raw)})
	_ = controller.Flush()
}

func (s *Server) accountPreview(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	accountPreviewResponse(w, r, s.runAccountPreview, 165*time.Second, 5*time.Second)
}
