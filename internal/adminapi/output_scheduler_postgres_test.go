package adminapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestOutputSchedulerRealOverlappingRefreshes(t *testing.T) {
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
			fmt.Fprint(w, `{"success":true,"obj":[]}`)
			return
		}
		t.Errorf("unexpected panel endpoint %s", r.URL.Path)
		w.WriteHeader(404)
	}))
	defer remote.Close()
	for i := 0; i < 36; i++ {
		seedPanel(t, db, fmt.Sprintf("%s/%d/", remote.URL, i))
	}
	sqlMust(t, db, "UPDATE deployments SET host='fixture.invalid'")
	eligible, err := (readyworker.SQLSource{DB: db}).EligibleReadyPanels(context.Background())
	if err != nil || len(eligible) != 36 {
		t.Fatalf("eligible fixture=%d: %v", len(eligible), err)
	}
	s := New(db, app.Container{})
	s.OutputRuntimes = &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: db, Secrets: panelTestSecrets{}, Timeout: 5 * time.Second}, TTL: time.Minute}
	defer func() {
		deadline := time.Now().Add(6 * time.Second)
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
	}()
	for tick := 0; tick < 6; tick++ {
		s.refreshOutputLive(context.Background())
		probe, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		if e := db.PingContext(probe); e != nil {
			t.Error("scheduler starved API SQL", e)
		}
		cancel()
		time.Sleep(time.Second)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 36 {
		t.Errorf("real overlapping collectors reached %d/36 panels", len(seen))
	}
	if peak < 2 || peak > 8 {
		t.Errorf("real collector concurrency=%d", peak)
	}
}
