package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestTransportPolicyPinsAndConcurrency(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	s := Server{DB: db}
	_, _, panel := seedPanel(t, db, "http://transport.test")
	_, _, other := seedPanel(t, db, "http://other.test")
	initial, e := transportprofile.Read(ctx, db)
	if e != nil || initial.Config.NetworkName() != "tcp" {
		t.Fatal(initial, e)
	}
	save := func(p transportprofile.Policy) int {
		b, _ := json.Marshal(p)
		w := httptest.NewRecorder()
		s.putConfigTransport(w, httptest.NewRequest("PUT", "/api/v1/configs/transport", bytes.NewReader(b)))
		return w.Code
	}
	if save(transportprofile.Policy{Revision: initial.Revision}) != 400 {
		t.Fatal("omitted network accepted")
	}
	wrong := transportprofile.Preset()
	wrong.Host = "example.org"
	if save(transportprofile.Policy{Revision: initial.Revision, Config: wrong}) != 400 {
		t.Fatal("wrong host accepted")
	}
	p := transportprofile.Policy{Revision: initial.Revision, Config: transportprofile.Preset()}
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- save(p) }()
	}
	wg.Wait()
	close(results)
	codes := map[int]int{}
	for c := range results {
		codes[c]++
	}
	if codes[200] != 1 || codes[409] != 1 {
		t.Fatal(codes)
	}
	legacy, e := transportprofile.Resolve(ctx, db, panel, "legacy", "tcp")
	if e != nil || legacy.NetworkName() != "tcp" {
		t.Fatal("legacy switched", e)
	}
	paths := make(chan string, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := transportprofile.Resolve(ctx, db, panel, "shared", "")
			paths <- c.Path
			errs <- e
		}()
	}
	wg.Wait()
	close(paths)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first string
	for v := range paths {
		if first == "" {
			first = v
		}
		if v != first {
			t.Fatal("concurrent path churn")
		}
	}
	c, e := transportprofile.Resolve(ctx, db, other, "shared", "")
	if e != nil || c.Path == first {
		t.Fatal("account/panel path collision", e)
	}
	if _, e = transportprofile.Resolve(ctx, db, panel, "shared", "tcp"); e == nil {
		t.Fatal("reinterpreted existing TCP")
	}
	// New default must not alter any already-pinned listener or the legacy TCP.
	if save(transportprofile.Policy{Revision: 2, Config: transportprofile.Config{Network: "tcp"}}) != 200 {
		t.Fatal("rollback default")
	}
	c, e = transportprofile.Resolve(ctx, db, panel, "shared", "xhttp")
	if e != nil || c.Path != first {
		t.Fatal("pin drift", e)
	}
	c, e = transportprofile.Resolve(ctx, db, panel, "new-tcp", "")
	if e != nil || c.NetworkName() != "tcp" {
		t.Fatal(e)
	}
	// Database constraint is the final cross-process path collision guard.
	if _, e = db.Exec(`INSERT INTO reality_listener_profiles(panel_id,managed_key,config,path,policy_revision) SELECT panel_id,'duplicate',config,path,policy_revision FROM reality_listener_profiles WHERE panel_id=$1 AND managed_key='shared'`, panel); e == nil {
		t.Fatal("duplicate path persisted")
	}
	for _, mutate := range []func(*transportprofile.Config){func(c *transportprofile.Config) { c.FinalMaskEnabled = true }, func(c *transportprofile.Config) { c.BrowserDialer = "WebView" }, func(c *transportprofile.Config) { c.Flow = "xtls-rprx-vision" }} {
		c := transportprofile.Preset()
		mutate(&c)
		if save(transportprofile.Policy{Revision: 3, Config: c}) != 400 {
			t.Fatal("invalid transport saved")
		}
	}
	c, e = transportprofile.Resolve(ctx, db, panel, "shared", "xhttp")
	if e != nil || c.Path != first {
		t.Fatal("invalid save changed pin")
	}
}
