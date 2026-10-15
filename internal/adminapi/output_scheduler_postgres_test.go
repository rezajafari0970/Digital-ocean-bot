package adminapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestOutputSchedulerRealOverlappingRefreshes(t *testing.T) {
	testOutputScheduler(t, false, false)
}
func TestOutputSchedulerSkipsBusyLocks(t *testing.T) {
	testOutputScheduler(t, true, false)
}
func TestOutputSchedulerRefillsSlotsWithinSweep(t *testing.T) {
	testOutputScheduler(t, true, true)
}
func testOutputScheduler(t *testing.T, lockEarly, singleSweep bool) {
	db := adminTestDB(t)
	db.SetMaxOpenConns(24)
	var mu sync.Mutex
	active, peak := 0, 0
	seen := map[string]bool{}
	inFlight := map[string]bool{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/csrf-token") {
			fmt.Fprint(w, `{"success":true,"obj":"csrf"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/login") {
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/panel/api/inbounds/list") {
			id := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
			mu.Lock()
			if inFlight[id] {
				t.Error("duplicate simultaneous refresh", id)
			}
			inFlight[id] = true
			active++
			if active > peak {
				peak = active
			}
			seen[id] = true
			mu.Unlock()
			delay := 400 * time.Millisecond
			if id == "0" {
				delay = 3 * time.Second
			}
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
			}
			mu.Lock()
			active--
			delete(inFlight, id)
			mu.Unlock()
			fmt.Fprint(w, `{"success":true,"obj":[{"id":1,"enable":true,"remark":"fixture","protocol":"vless","port":443,"settings":{"decryption":"none","clients":[{"id":"b530c072-21c6-405d-95d8-fa78afa2d62e","enable":true,"email":"test","flow":"xtls-rprx-vision"}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"serverNames":["www.example.com"],"shortIds":["aabb"]}}}]}`)
			return
		}
		t.Errorf("unexpected panel endpoint %s", r.URL.Path)
		w.WriteHeader(404)
	}))
	defer remote.Close()
	for i := 0; i < 36; i++ {
		_, _, panel := seedPanel(t, db, fmt.Sprintf("%s/%d/", remote.URL, i))
		sqlMust(t, db, "INSERT INTO inbound_export_metadata(panel_id,remote_id,public_key) VALUES($1,1,'public')", panel)
	}
	sqlMust(t, db, "UPDATE deployments SET host='fixture.invalid'")
	eligible, err := (readyworker.SQLSource{DB: db}).EligibleReadyPanels(context.Background())
	if err != nil || len(eligible) != 36 {
		t.Fatalf("eligible fixture=%d: %v", len(eligible), err)
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	blocker, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	defer blocker.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()")
	if lockEarly {
		for _, p := range eligible[:8] {
			if _, err := blocker.ExecContext(context.Background(), "SELECT pg_advisory_lock(hashtextextended($1,0))", "panel-config:"+p.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	s := New(db, app.Container{})
	s.OutputRuntimes = &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: panelTestSecrets{}, Timeout: 5 * time.Second}, TTL: time.Minute}
	drain := func() {
		deadline := time.Now().Add(7 * time.Second)
		for time.Now().Before(deadline) {
			s.OutputPanelMu.Lock()
			n := len(s.OutputPanelRun)
			s.OutputPanelMu.Unlock()
			if n == 0 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("bounded collectors did not drain")
	}
	defer drain()
	// Probe pool headroom while the dispatcher itself is waiting/refilling.
	probeStop, probeDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(probeDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-probeStop:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
				if err := db.PingContext(ctx); err != nil {
					t.Error("dispatch starved concurrent SQL", err)
				}
				cancel()
			}
		}
	}()
	defer func() { close(probeStop); <-probeDone }()
	runTicks := func() {
		ticks := 6
		if singleSweep {
			ticks = 1
		}
		for tick := 0; tick < ticks; tick++ {
			s.refreshOutputLive(context.Background())
			probe, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			if e := db.PingContext(probe); e != nil {
				t.Error("scheduler starved API SQL", e)
			}
			cancel()
			time.Sleep(time.Second)
		}
	}
	assertFresh := func(want int) {
		t.Helper()
		var count int
		if err := db.QueryRow("SELECT count(DISTINCT panel_id) FROM output_config_snapshots WHERE last_seen_at>now()-interval '15 seconds'").Scan(&count); err != nil || count != want {
			t.Fatalf("completed fresh snapshots=%d want=%d err=%v", count, want, err)
		}
	}
	runTicks()
	drain()
	if lockEarly {
		assertFresh(28)
		if _, err := blocker.ExecContext(context.Background(), "SELECT pg_advisory_unlock_all()"); err != nil {
			t.Fatal(err)
		}
		runTicks()
		drain()
	}
	assertFresh(36)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 36 {
		t.Errorf("real overlapping collectors reached %d/36 panels", len(seen))
	}
	if peak < 2 || peak > 8 {
		t.Errorf("real collector concurrency=%d", peak)
	}
}
