package sanaei

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A canceled caller must not wait behind another caller's network read or
// report that caller's cached result as a successful observation.
func TestPanelSessionCanceledReadersDoNotWaitForNetwork(t *testing.T) {
	for _, method := range []string{"Snapshot", "RawInbound", "Index", "FreshSnapshot", "FreshRawInbound", "InvalidateContext"} {
		t.Run(method, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				once.Do(func() { close(entered) })
				<-release
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"obj":[{"id":7,"remark":"r","port":443}]}`))
			}))
			defer srv.Close()
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			session := NewPanelSession(&APIClient{BaseURL: srv.URL, HTTP: srv.Client()})
			first := make(chan error, 1)
			go func() { _, err := session.Snapshot(context.Background()); first <- err }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("first read did not reach server")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			result := make(chan error, 1)
			go func() {
				var err error
				switch method {
				case "Snapshot":
					_, err = session.Snapshot(ctx)
				case "RawInbound":
					_, _, err = session.RawInbound(ctx, 7)
				case "Index":
					_, err = session.Index(ctx)
				case "FreshSnapshot":
					_, err = session.FreshSnapshot(ctx)
				case "FreshRawInbound":
					_, _, err = session.FreshRawInbound(ctx, 7)
				case "InvalidateContext":
					err = session.InvalidateContext(ctx)
				}
				result <- err
			}()
			returned := false
			select {
			case err := <-result:
				returned = true
				if !errors.Is(err, context.Canceled) {
					t.Errorf("canceled reader returned %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				t.Error("canceled reader waited for another caller's network request")
			}
			unblock()
			if err := <-first; err != nil {
				t.Errorf("original read changed: %v", err)
			}
			if !returned {
				<-result
			}
			if _, err := session.Snapshot(context.Background()); err != nil {
				t.Errorf("canceled waiter damaged original cache: %v", err)
			}
		})
	}
}

func TestPanelSessionCanceledReaderDoesNotUseCachedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":[]}`))
	}))
	defer srv.Close()
	session := NewPanelSession(&APIClient{BaseURL: srv.URL, HTTP: srv.Client()})
	if _, err := session.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := session.Snapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cached read returned %v", err)
	}
}

func TestPanelSessionDeadlineExpiresWhileWaitingForAnotherRead(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":[]}`))
	}))
	defer srv.Close()
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	session := NewPanelSession(&APIClient{BaseURL: srv.URL, HTTP: srv.Client()})
	first := make(chan error, 1)
	go func() { _, err := session.Snapshot(context.Background()); first <- err }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := session.Snapshot(ctx); result <- err }()
	<-ctx.Done()
	returned := false
	select {
	case err := <-result:
		returned = true
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("deadline returned %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expired waiter stayed behind another network read")
	}
	unblock()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if !returned {
		<-result
	}
}

func TestPanelSessionCanceledLoadDoesNotPoisonNextReader(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":[{"id":7,"remark":"r","port":443}]}`))
	}))
	defer srv.Close()
	session := NewPanelSession(&APIClient{BaseURL: srv.URL, HTTP: srv.Client()})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := session.Snapshot(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first load: %v", err)
	}
	raw, ok, err := session.RawInbound(context.Background(), 7)
	if err != nil || !ok || len(raw) == 0 || requests.Load() != 2 {
		t.Fatalf("live reader poisoned: found=%v requests=%d err=%v", ok, requests.Load(), err)
	}
}

func TestPanelSessionFreshReadAndIndexStayPaired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":[{"id":7,"remark":"r","port":443}]}`))
	}))
	defer srv.Close()
	session := NewPanelSession(&APIClient{BaseURL: srv.URL, HTTP: srv.Client()})
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 25; i++ {
				session.Invalidate()
				if _, ok, err := session.RawInbound(context.Background(), 7); err != nil || !ok {
					t.Errorf("raw/index detached from loaded snapshot: %v %v", ok, err)
				}
				if index, err := session.Index(context.Background()); err != nil || index == nil || len(index.ByRemoteID[7]) == 0 {
					t.Errorf("index detached: %v", err)
				}
				if _, ok, err := session.FreshRawInbound(context.Background(), 7); err != nil || !ok {
					t.Errorf("fresh raw: %v %v", ok, err)
				}
			}
		}()
	}
	workers.Wait()
}
