package sanaei

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fastPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
		MaxDelay:    2 * time.Millisecond,
	}
}

func TestResilientRetries5xx(t *testing.T) {
	var calls atomic.Int32

	server :=
		httptest.NewServer(
			http.HandlerFunc(
				func(
					w http.ResponseWriter,
					r *http.Request,
				) {
					if calls.Add(1) < 3 {
						w.WriteHeader(
							http.StatusServiceUnavailable,
						)
						return
					}

					w.WriteHeader(
						http.StatusOK,
					)

					_, _ =
						w.Write(
							[]byte(
								`{"success":true}`,
							),
						)
				},
			),
		)

	defer server.Close()

	client :=
		&APIClient{
			BaseURL: server.URL,
			HTTP:    server.Client(),
		}

	session :=
		&ResilientSession{
			Client: client,
			Policy: fastPolicy(),
		}

	resp, err :=
		session.Do(
			context.Background(),
			SessionRequest{
				Method: http.MethodGet,
				Path:   "test",
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode !=
		http.StatusOK {

		t.Fatalf(
			"status=%d",
			resp.StatusCode,
		)
	}

	if calls.Load() != 3 {
		t.Fatalf(
			"calls=%d",
			calls.Load(),
		)
	}
}

func TestResilientRetries429(t *testing.T) {
	var calls atomic.Int32

	server :=
		httptest.NewServer(
			http.HandlerFunc(
				func(
					w http.ResponseWriter,
					r *http.Request,
				) {
					if calls.Add(1) == 1 {
						w.WriteHeader(
							http.StatusTooManyRequests,
						)
						return
					}

					w.WriteHeader(
						http.StatusOK,
					)
				},
			),
		)

	defer server.Close()

	session :=
		&ResilientSession{
			Client: &APIClient{
				BaseURL: server.URL,
				HTTP:    server.Client(),
			},
			Policy: fastPolicy(),
		}

	resp, err :=
		session.Do(
			context.Background(),
			SessionRequest{
				Method: http.MethodGet,
				Path:   "test",
			},
		)

	if err != nil ||
		resp.StatusCode != 200 {

		t.Fatalf(
			"status=%d err=%v",
			resp.StatusCode,
			err,
		)
	}

	if calls.Load() != 2 {
		t.Fatalf(
			"calls=%d",
			calls.Load(),
		)
	}
}

func TestResilientFailsFast400(t *testing.T) {
	var calls atomic.Int32

	server :=
		httptest.NewServer(
			http.HandlerFunc(
				func(
					w http.ResponseWriter,
					r *http.Request,
				) {
					calls.Add(1)

					w.WriteHeader(
						http.StatusBadRequest,
					)
				},
			),
		)

	defer server.Close()

	session :=
		&ResilientSession{
			Client: &APIClient{
				BaseURL: server.URL,
				HTTP:    server.Client(),
			},
			Policy: fastPolicy(),
		}

	resp, err :=
		session.Do(
			context.Background(),
			SessionRequest{
				Method: http.MethodPost,
				Path:   "test",
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 400 {
		t.Fatalf(
			"status=%d",
			resp.StatusCode,
		)
	}

	if calls.Load() != 1 {
		t.Fatalf(
			"unexpected retries=%d",
			calls.Load(),
		)
	}
}

func TestResilientHonorsCancellation(
	t *testing.T,
) {
	ctx, cancel :=
		context.WithCancel(
			context.Background(),
		)

	cancel()

	session :=
		&ResilientSession{
			Client: &APIClient{
				BaseURL: "http://127.0.0.1:1",
				HTTP:    &http.Client{},
			},
			Policy: fastPolicy(),
		}

	_, err :=
		session.Do(
			ctx,
			SessionRequest{
				Method: http.MethodGet,
				Path:   "test",
			},
		)

	if err == nil {
		t.Fatal(
			"expected cancellation",
		)
	}
}

func TestReadAllBounded(t *testing.T) {
	data :=
		bytes.Repeat(
			[]byte("x"),
			1025,
		)

	if _, err :=
		ReadAllBounded(
			bytes.NewReader(data),
			1024,
		); err == nil {

		t.Fatal(
			"expected size rejection",
		)
	}

	ok :=
		bytes.Repeat(
			[]byte("x"),
			1024,
		)

	got, err :=
		ReadAllBounded(
			bytes.NewReader(ok),
			1024,
		)

	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1024 {
		t.Fatalf(
			"len=%d",
			len(got),
		)
	}
}

func TestRetryableErrorWithContextClientTimeout(t *testing.T) {
	ctx := context.Background()
	if !retryableErrorWithContext(ctx, context.DeadlineExceeded) {
		t.Fatal("client timeout should retry while parent is alive")
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if retryableErrorWithContext(cctx, context.DeadlineExceeded) {
		t.Fatal("parent cancellation must stop retries")
	}
}

func TestResilientDoesNotBlindRetryMutation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	session := &ResilientSession{Client: &APIClient{BaseURL: server.URL, HTTP: server.Client()}, Policy: fastPolicy()}
	_, _ = session.Do(context.Background(), SessionRequest{Method: http.MethodPost, Path: "panel/api/inbounds/add"})
	if calls.Load() != 1 {
		t.Fatalf("mutation calls=%d want=1", calls.Load())
	}
}

func TestResilientStillRetriesSafeRead(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	session := &ResilientSession{Client: &APIClient{BaseURL: server.URL, HTTP: server.Client()}, Policy: fastPolicy()}
	resp, err := session.Do(context.Background(), SessionRequest{Method: http.MethodGet, Path: "panel/api/inbounds/list"})
	if err != nil || resp.StatusCode != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("status=%d calls=%d err=%v", resp.StatusCode, calls.Load(), err)
	}
}
