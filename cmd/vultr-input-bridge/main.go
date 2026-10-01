package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type pointRequest struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	DeltaY float64 `json:"delta_y,omitempty"`
}

func vultrTab(root context.Context) (context.Context, context.CancelFunc, error) {
	ctx, stop := context.WithTimeout(root, 2*time.Second)
	defer stop()
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return nil, nil, err
	}
	var id target.ID
	for _, t := range targets {
		if t.Type == "page" && (strings.Contains(t.URL, "vultr.com") || strings.Contains(t.Title, "Vultr")) {
			id = t.TargetID
			break
		}
	}
	if id == "" {
		return nil, nil, errors.New("vultr target unavailable")
	}
	tab, cancel := chromedp.NewContext(root, chromedp.WithTargetID(id))
	return tab, cancel, nil
}

func decodePoint(w http.ResponseWriter, r *http.Request) (pointRequest, bool) {
	var req pointRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return req, false
	}
	req.X = max(0, min(411, req.X))
	req.Y = max(0, min(914, req.Y))
	return req, true
}

func main() {
	root, cancel := chromedp.NewRemoteAllocator(context.Background(), "http://127.0.0.1:19223")
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /scroll", func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePoint(w, r)
		if !ok {
			return
		}
		req.DeltaY = max(-900, min(900, req.DeltaY))
		tab, tabCancel, err := vultrTab(root)
		if err != nil {
			http.Error(w, "browser unavailable", http.StatusServiceUnavailable)
			return
		}
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

	mux.HandleFunc("POST /tap", func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePoint(w, r)
		if !ok {
			return
		}
		tab, tabCancel, err := vultrTab(root)
		if err != nil {
			http.Error(w, "browser unavailable", http.StatusServiceUnavailable)
			return
		}
		defer tabCancel()
		runCtx, runCancel := context.WithTimeout(tab, 2*time.Second)
		defer runCancel()
		if err := chromedp.Run(runCtx,
			input.DispatchMouseEvent(input.MousePressed, req.X, req.Y).
				WithButton(input.Left).WithButtons(1).WithClickCount(1),
			input.DispatchMouseEvent(input.MouseReleased, req.X, req.Y).
				WithButton(input.Left).WithButtons(0).WithClickCount(1),
		); err != nil {
			http.Error(w, "tap failed", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{Addr: "127.0.0.1:16081", Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
