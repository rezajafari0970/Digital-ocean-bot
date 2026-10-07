package supervision

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartLedgerPersistsPacingAndExclusiveOwner(t *testing.T) {
	p := filepath.Join(t.TempDir(), "role.json")
	g := RestartGate{Path: p}
	if e := g.Enter(context.Background()); e != nil {
		t.Fatal(e)
	}
	second := RestartGate{Path: p}
	if second.Enter(context.Background()) == nil {
		t.Fatal("duplicate owner admitted")
	}
	g.Close()
	if e := second.Enter(context.Background()); e != nil {
		t.Fatal(e)
	}
	s, e := second.read()
	if e != nil || s.Attempts != 2 || s.NextStart <= s.LastStart {
		t.Fatal(s, e)
	}
	second.Close()
	third := RestartGate{Path: p}
	defer third.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e = third.Enter(ctx); e == nil {
		t.Fatal("restart pacing lost")
	}
	s, e = third.read()
	if e != nil || s.Attempts != 2 {
		t.Fatal("canceled wait advanced attempt", s, e)
	}
	if e = third.Healthy(); e != nil {
		t.Fatal(e)
	}
	s, e = third.read()
	if e != nil || s.Attempts != 0 || s.NextStart != 0 {
		t.Fatal("healthy reset failed", s, e)
	}
}
func TestRestartLedgerMalformedAndDelayCap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "role.json")
	for _, raw := range []string{"bad", "{}", `{"version":1,"attempts":-1}`, `{"version":9,"attempts":0}`} {
		if e := os.WriteFile(p, []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
		g := RestartGate{Path: p}
		if g.Enter(context.Background()) == nil {
			t.Fatal("malformed ledger accepted")
		}
		g.Close()
	}
	for i := 1; i < 100; i++ {
		d := RestartDelay(i)
		if d < 0 || d > 10*time.Minute {
			t.Fatal(i, d)
		}
	}
}

func TestRestartHistorySurvivesLongDowntime(t *testing.T) {
	g := &RestartGate{Path: filepath.Join(t.TempDir(), "control.json")}
	if err := g.write(RestartState{Version: 1, Attempts: 7, LastStart: time.Now().Add(-24 * time.Hour).Unix(), NextStart: time.Now().Add(-time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	if err := g.Enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	s, err := g.read()
	if err != nil || s.Attempts != 8 || time.Until(time.Unix(s.NextStart, 0)) < 9*time.Minute {
		t.Fatal("crash history reset by downtime", s, err)
	}
}
func TestRestartFutureClockDoesNotResetHistory(t *testing.T) {
	g := &RestartGate{Path: filepath.Join(t.TempDir(), "control.json")}
	if err := g.write(RestartState{Version: 1, Attempts: 7, LastStart: time.Now().Add(24 * time.Hour).Unix(), NextStart: time.Now().Add(25 * time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := g.Enter(ctx); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	defer g.Close()
	s, err := g.read()
	if err != nil || s.Attempts != 7 {
		t.Fatal("clock regression erased history", s, err)
	}
}
