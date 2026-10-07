package supervision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type RestartState struct {
	Version   int   `json:"version"`
	Attempts  int   `json:"attempts"`
	LastStart int64 `json:"last_start_unix"`
	NextStart int64 `json:"next_start_unix"`
}
type RestartGate struct {
	mu   sync.Mutex
	Path string
	lock *os.File
}

func RestartDelay(attempt int) time.Duration {
	if attempt < 2 {
		return 0
	}
	if attempt > 7 {
		return 10 * time.Minute
	}
	return time.Duration(1<<uint(attempt-2)) * 15 * time.Second
}
func (g *RestartGate) read() (RestartState, error) {
	f, e := os.Open(g.Path)
	if errors.Is(e, os.ErrNotExist) {
		return RestartState{Version: 1}, nil
	}
	if e != nil {
		return RestartState{}, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(raw) > 4096 {
		return RestartState{}, ErrProtocol
	}
	var s RestartState
	if json.Unmarshal(raw, &s) != nil || s.Version != 1 || s.Attempts < 0 || s.Attempts > 1000000 || s.LastStart < 0 || s.NextStart < 0 {
		return s, ErrProtocol
	}
	return s, nil
}
func (g *RestartGate) write(s RestartState) error {
	dir := filepath.Dir(g.Path)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	raw, e := json.Marshal(s)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, "restart-")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = os.Rename(temp, g.Path); e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// A saved future cooldown is capped conservatively after wall-clock rollback.
// Pacing runs BEFORE SQL ownership acquisition and holds no pool connection.
func (g *RestartGate) Enter(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(g.Path), 0700); err != nil {
		return err
	}
	if g.lock == nil {
		lock, err := os.OpenFile(g.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			lock.Close()
			return fmt.Errorf("restart role already owned: %w", err)
		}
		g.lock = lock
	}
	s, e := g.read()
	if e != nil {
		return fmt.Errorf("restart ledger unreadable: %w", e)
	}
	attempt := s.Attempts + 1
	if attempt > 100 {
		attempt = 100
	}
	delay := RestartDelay(attempt)
	wait := time.Until(time.Unix(s.NextStart, 0))
	if wait > 10*time.Minute {
		wait = 10 * time.Minute
	}
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Write the next earliest restart before any native work can begin.
	s = RestartState{Version: 1, Attempts: attempt, LastStart: time.Now().Unix(), NextStart: time.Now().Add(delay).Unix()}
	return g.write(s)
}
func (g *RestartGate) Healthy() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, e := g.read()
	if e != nil {
		return e
	}
	s.Attempts = 0
	s.NextStart = 0
	return g.write(s)
}

func (g *RestartGate) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lock != nil {
		_ = syscall.Flock(int(g.lock.Fd()), syscall.LOCK_UN)
		_ = g.lock.Close()
		g.lock = nil
	}
}
