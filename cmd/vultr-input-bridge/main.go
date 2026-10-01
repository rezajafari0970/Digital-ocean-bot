package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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

type chromeVersion struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func browserWebSocketURL() (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:19223/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("chrome debugging endpoint unavailable")
	}
	var version chromeVersion
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return "", err
	}
	if !strings.HasPrefix(version.WebSocketDebuggerURL, "ws://127.0.0.1:19223/") {
		return "", errors.New("unexpected chrome debugger URL")
	}
	return version.WebSocketDebuggerURL, nil
}

type chromeTarget struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func vultrTargetID() (target.ID, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:19223/json/list")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var targets []chromeTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", err
	}
	for _, t := range targets {
		if t.Type == "page" && (strings.Contains(t.URL, "vultr.com") || strings.Contains(t.Title, "Vultr")) {
			return target.ID(t.ID), nil
		}
	}
	return "", errors.New("vultr target unavailable")
}

func vultrTab(alloc context.Context) (context.Context, context.CancelFunc, error) {
	id, err := vultrTargetID()
	if err != nil {
		return nil, nil, err
	}
	tab, cancel := chromedp.NewContext(alloc, chromedp.WithTargetID(id))
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
	wsURL, err := browserWebSocketURL()
	if err != nil {
		panic(err)
	}
	alloc, cancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /scroll", func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePoint(w, r)
		if !ok {
			return
		}
		req.DeltaY = max(-900, min(900, req.DeltaY))
		tab, tabCancel, err := vultrTab(alloc)
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
			log.Printf("scroll dispatch failed: %v", err)
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
		tab, tabCancel, err := vultrTab(alloc)
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
