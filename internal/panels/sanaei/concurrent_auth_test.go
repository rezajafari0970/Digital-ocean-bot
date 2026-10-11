package sanaei

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// In production the relay observer already shares a session with routing.
// Exercise changing CSRF state while native read requests use that session.
func TestConcurrentLoginAndNativeReads(t *testing.T) {
	var tokens atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/csrf-token" {
			fmt.Fprintf(w, `{"success":true,"obj":"token-%d"}`, tokens.Add(1))
			return
		}
		if r.URL.Path == "/read" && r.Header.Get("X-CSRF-Token") == "" {
			t.Error("missing CSRF")
		}
		fmt.Fprint(w, `{"success":true}`)
	}))
	defer srv.Close()
	client := &APIClient{BaseURL: srv.URL, HTTP: srv.Client(), Credentials: Credentials{Username: "test", Password: "test"}}
	if err := client.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				if i == 0 {
					if err := client.Login(context.Background()); err != nil {
						t.Error(err)
					}
				} else if _, err := (DirectSessionExecutor{Client: client}).Do(context.Background(), SessionRequest{Method: "GET", Path: "read"}); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestNativeReadCancellationWhileLoginOwnsSession(t *testing.T) {
	client := &APIClient{BaseURL: "http://127.0.0.1", HTTP: &http.Client{}}
	client.authMu.Lock()
	defer client.authMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (DirectSessionExecutor{Client: client}).Do(ctx, SessionRequest{Method: "GET", Path: "read"})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled read waited for login ownership")
	}
}
func TestConcurrentExpiredReadsReloginOnce(t *testing.T) {
	var initial, logins atomic.Int32
	barrier := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/csrf-token":
			fmt.Fprint(w, `{"success":true,"obj":"new-token"}`)
		case "/login":
			logins.Add(1)
			fmt.Fprint(w, `{"success":true}`)
		default:
			if r.Header.Get("X-CSRF-Token") == "" {
				if initial.Add(1) == 4 {
					close(barrier)
				}
				<-barrier
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"success":true}`)
		}
	}))
	defer srv.Close()
	session := &ResilientSession{Client: &APIClient{BaseURL: srv.URL, HTTP: srv.Client()}, Policy: fastPolicy()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := session.Do(ctx, SessionRequest{Method: "GET", Path: "read"})
			if err != nil || resp.StatusCode != 200 {
				t.Error(resp.StatusCode, err)
			}
		}()
	}
	wg.Wait()
	if logins.Load() != 1 {
		t.Fatal("duplicate login burst", logins.Load())
	}
}
