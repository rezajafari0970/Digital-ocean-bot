package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type scrollRequest struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	DeltaY float64 `json:"delta_y"`
}

func main() {
	root, cancel := chromedp.NewRemoteAllocator(context.Background(), "http://127.0.0.1:19223")
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /scroll", func(w http.ResponseWriter, r *http.Request) {
		var req scrollRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.DeltaY > 900 {
			req.DeltaY = 900
		}
		if req.DeltaY < -900 {
			req.DeltaY = -900
		}

		ctx, stop := context.WithTimeout(root, 2*time.Second)
		defer stop()
		targets, err := chromedp.Targets(ctx)
		if err != nil {
			http.Error(w, "browser unavailable", http.StatusServiceUnavailable)
			return
		}
		var targetID target.ID
		for _, t := range targets {
			if t.Type == "page" && (strings.Contains(t.URL, "vultr.com") || strings.Contains(t.Title, "Vultr")) {
				targetID = t.TargetID
				break
			}
		}
		if targetID == "" {
			http.Error(w, "vultr target unavailable", http.StatusServiceUnavailable)
			return
		}

		tab, tabCancel := chromedp.NewContext(root, chromedp.WithTargetID(targetID))
		defer tabCancel()
		runCtx, runCancel := context.WithTimeout(tab, 2*time.Second)
		defer runCancel()
		if err := chromedp.Run(runCtx,
			input.DispatchMouseEvent(input.MouseWheel, req.X, req.Y).WithDeltaY(req.DeltaY),
		); err != nil {
			http.Error(w, "scroll failed", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{
		Addr:              "127.0.0.1:16081",
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
