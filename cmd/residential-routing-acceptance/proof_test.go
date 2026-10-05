package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRouteProofDistinguishesUnavailableFromMismatch(t *testing.T) {
	for _, tc := range []struct {
		name               string
		status             int
		body               string
		match, unavailable bool
	}{
		{"valid", 200, `{"success":true,"obj":{"matched":true,"outboundTag":"expected"}}`, true, false},
		{"different outbound", 200, `{"success":true,"obj":{"matched":true,"outboundTag":"other"}}`, false, false},
		{"unmatched", 200, `{"success":true,"obj":{"matched":false}}`, false, false},
		{"closed RPC", 200, `{"success":false,"msg":"rpc error: code = Canceled desc = grpc: the client connection is closing"}`, false, true},
		{"bad envelope", 200, `<html>login</html>`, false, true},
		{"HTTP failure", 503, `{"success":true,"obj":{"matched":true,"outboundTag":"expected"}}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			match, err := routeProofMatches(sanaei.SessionResponse{StatusCode: tc.status, Body: []byte(tc.body)}, "expected")
			if match != tc.match || (err != nil) != tc.unavailable {
				t.Fatalf("match=%v err=%v", match, err)
			}
		})
	}
}

func TestProofUsesWorkerLockAndKeepsOtherPanelsIndependent(t *testing.T) {
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test database required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "test") {
		t.Fatal("refusing non-test database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	panel := fmt.Sprintf("acceptance-proof-%d", time.Now().UnixNano())
	held := make(chan struct{})
	release := make(chan struct{})
	workerDone := make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	go func() {
		workerDone <- sanaei.WithConfigLock(ctx, db, panel, func(context.Context) error {
			close(held)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-held:
	case err := <-workerDone:
		t.Fatalf("worker lock: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var canceledCalls atomic.Int32
	canceled, cancelWait := context.WithTimeout(ctx, 80*time.Millisecond)
	_, err = verifyUnderConfigLock(canceled, db, panel, func(context.Context) (int, error) { canceledCalls.Add(1); return 34, nil })
	cancelWait()
	if err == nil || canceledCalls.Load() != 0 {
		t.Fatal("canceled waiter performed proof")
	}
	n, err := verifyUnderConfigLock(ctx, db, panel+"-other", func(context.Context) (int, error) { return 34, nil })
	if err != nil || n != 34 {
		t.Fatalf("another panel blocked: n=%d err=%v", n, err)
	}
	entered := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		n, e := verifyUnderConfigLock(ctx, db, panel, func(context.Context) (int, error) { entered <- struct{}{}; return 34, nil })
		if e == nil && n != 34 {
			e = fmt.Errorf("invalid proof count %d", n)
		}
		done <- e
	}()
	select {
	case <-entered:
		t.Fatal("proof overlapped worker lock")
	case e := <-done:
		t.Fatalf("proof ended before worker released: %v", e)
	case <-time.After(80 * time.Millisecond):
	}
	unblock()
	if err = <-workerDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-entered:
	default:
		t.Fatal("proof did not execute after worker released")
	}
}
