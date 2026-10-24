package adminapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountPreviewStreamSurvivesServerWriteTimeout(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountPreviewResponse(w, r, func(w http.ResponseWriter, r *http.Request) {
			previewStage(r.Context(), "catalog")
			select {
			case <-time.After(160 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			writeJSON(w, 200, map[string]any{"provider": "aws", "regions": []string{"eu-west-1", "us-east-1"}})
		}, time.Second, 20*time.Millisecond)
	}))
	server.Config.WriteTimeout = 40 * time.Millisecond
	server.Start()
	defer server.Close()
	req, _ := http.NewRequest("POST", server.URL, strings.NewReader("{}"))
	req.Header.Set("Accept", previewStreamType)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("buffered preview")
	}
	scan := bufio.NewScanner(res.Body)
	progress, result := 0, 0
	for scan.Scan() {
		var event struct {
			Type, Stage string
			Status      int
			Body        json.RawMessage
		}
		if err = json.Unmarshal(scan.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "progress" {
			progress++
		}
		if event.Type == "result" {
			result++
			if event.Status != 200 || !strings.Contains(string(event.Body), "eu-west-1") {
				t.Fatal(string(scan.Bytes()))
			}
		}
	}
	if scan.Err() != nil || progress < 2 || result != 1 {
		t.Fatal(progress, result, scan.Err())
	}
}
func TestAccountPreviewStreamCancellationAndDeadline(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			returned := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				accountPreviewResponse(w, r, func(w http.ResponseWriter, r *http.Request) {
					<-r.Context().Done()
					writeJSON(w, 500, map[string]string{"error": "internal"})
					close(returned)
				}, 70*time.Millisecond, 10*time.Millisecond)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader("{}"))
			req.Header.Set("Accept", previewStreamType)
			res, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if kind == "cancel" {
				cancel()
			} else {
				raw, e := io.ReadAll(res.Body)
				if e != nil || !strings.Contains(string(raw), `"status":504`) || !strings.Contains(string(raw), "provider_preview_timeout") {
					t.Fatal(string(raw), e)
				}
			}
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("provider work not cancelled")
			}
		})
	}
}
func TestAccountPreviewStreamPreservesErrorsAndRejectsIncompleteResults(t *testing.T) {
	for _, status := range []int{401, 403, 422, 429, 503, 0} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
			r.Header.Set("Accept", previewStreamType)
			accountPreviewResponse(w, r, func(w http.ResponseWriter, r *http.Request) {
				if status != 0 {
					writeJSON(w, status, map[string]string{"error": "provider_fixture"})
				}
			}, time.Second, 20*time.Millisecond)
			events := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
			var result struct {
				Type   string
				Status int
			}
			if err := json.Unmarshal([]byte(events[len(events)-1]), &result); err != nil {
				t.Fatal(err)
			}
			want := status
			if want == 0 {
				want = 502
			}
			if result.Type != "result" || result.Status != want {
				t.Fatal(w.Body.String())
			}
		})
	}
}
func TestAccountPreviewStreamBodyLimitAndJSONCompatibility(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 65537)))
	r.Header.Set("Accept", previewStreamType)
	accountPreviewResponse(w, r, func(http.ResponseWriter, *http.Request) { t.Fatal("oversized request accepted") }, time.Second, time.Second)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/", strings.NewReader("{}"))
	accountPreviewResponse(w, r, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("unbounded JSON request")
		}
		writeJSON(w, 422, map[string]string{"error": "fixture"})
	}, time.Second, time.Second)
	if w.Code != 422 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Code, w.Header())
	}
}
