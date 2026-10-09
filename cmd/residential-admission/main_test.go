package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmissionTransportInput(t *testing.T) {
	for _, v := range []string{"", "https://example.com", "vless://user@host:443?security=reality", "vless://11111111-1111-4111-8111-111111111111@host:443?security=tls"} {
		if _, err := parseTunnel(v); err == nil {
			t.Fatal("invalid transport accepted")
		}
	}
	v, err := parseTunnel("vless://11111111-1111-4111-8111-111111111111@127.0.0.1:443?security=reality&pbk=fixture&type=tcp&sni=fixture")
	if err != nil || v["protocol"] != "vless" {
		t.Fatal(err)
	}
	if _, err = ids("11111111-1111-4111-8111-111111111111,11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("duplicate role accepted")
	}
}
func TestAdmissionCurlFixedBudgetAndOutcomes(t *testing.T) {
	d := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$ADMISSION_TEST_ARGS"
printf '{"http_code":204}'
exit 0
`
	if err := os.WriteFile(filepath.Join(d, "curl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d)
	t.Setenv("ADMISSION_TEST_ARGS", filepath.Join(d, "args"))
	o := attempt(context.Background(), 1080, "https://example.invalid", 204)
	if o.Outcome != "ok" || o.HTTPStatus != 204 {
		t.Fatal(o)
	}
	b, _ := os.ReadFile(filepath.Join(d, "args"))
	args := string(b)
	for _, want := range []string{"--disable\n--retry\n0\n", "--head\n", "--connect-timeout\n6\n", "--max-time\n10\n", "--output\n/dev/null\n", "--socks5-hostname\n127.0.0.1:1080\n"} {
		if !strings.Contains(args, want) {
			t.Fatal("missing transport constraint", want)
		}
	}
	for _, deny := range []string{"--location", "--insecure"} {
		if strings.Contains(args, deny) {
			t.Fatal("unsafe curl option", deny)
		}
	}
	for code, want := range map[int]string{28: "timeout", 7: "local_proxy_unavailable", 35: "tls", 60: "tls", 67: "auth", 97: "transport"} {
		script = "#!/bin/sh\nprintf '{\"http_code\":0}'\nexit " + strconv.Itoa(code) + "\n"
		os.WriteFile(filepath.Join(d, "curl"), []byte(script), 0700)
		if got := attempt(context.Background(), 1080, "https://example.invalid", 204).Outcome; got != want {
			t.Fatal(code, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if o := attempt(ctx, 1080, "https://example.invalid", 204); o.Outcome != "not_started" {
		t.Fatal(o)
	}
}
func TestInstalledCoreAdmissionDialerGuardAndCleanup(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("explicit native core required")
	}
	d := t.TempDir()
	t.Setenv("TMPDIR", d)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Error("non-HEAD request")
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	// Local upstream SOCKS endpoint, with public-network access unnecessary.
	upstream, stop, err := startCore(ctx, binary, []map[string]any{{"tag": "upstream", "protocol": "freedom"}}, map[string]any{"tag": "vps-diagnostic", "protocol": "blackhole"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	socks := map[string]any{"tag": "residential-fixture", "protocol": "socks", "settings": map[string]any{"servers": []any{map[string]any{"address": "127.0.0.1", "port": upstream[0]}}}, "streamSettings": map[string]any{"sockopt": map[string]any{"dialerProxy": "vps-diagnostic"}}}
	for _, denied := range []bool{false, true} {
		protocol := "freedom"
		if denied {
			protocol = "blackhole"
		}
		ports, end, err := startCore(ctx, binary, []map[string]any{socks}, map[string]any{"tag": "vps-diagnostic", "protocol": protocol})
		if err != nil {
			t.Fatal(err)
		}
		o := attempt(ctx, ports[0], srv.URL, 204)
		if denied && ((o.CurlCode != 56 && o.CurlCode != 52) || o.HTTPStatus != 0) {
			t.Fatalf("unreviewed blackhole signature: %+v", o)
		}
		end()
		end()
		if (o.Outcome == "ok") == denied {
			t.Fatalf("dialer guard: denied=%v outcome=%s", denied, o.Outcome)
		}
		if c, e := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[0])), 50*time.Millisecond); e == nil {
			c.Close()
			t.Fatal("diagnostic listener survived cleanup")
		}
	}
	stop()
	entries, _ := os.ReadDir(d)
	if len(entries) != 0 {
		b, _ := json.Marshal(entries)
		t.Fatal("private configurations survived", string(b))
	}
}

func TestInstalledCoreAdmissionIgnoresHostileCurlConfig(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("explicit native core required")
	}
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "HEAD" {
			t.Error("method escaped HEAD")
		}
		w.Header().Set("Location", "/extra")
		w.WriteHeader(503)
	}))
	defer srv.Close()
	d := t.TempDir()
	t.Setenv("CURL_HOME", d)
	if err := os.WriteFile(filepath.Join(d, ".curlrc"), []byte("location\nretry = 3\ninsecure\nurl = "+srv.URL+"/extra\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ports, stop, err := startCore(ctx, binary, []map[string]any{{"tag": "upstream", "protocol": "freedom"}}, map[string]any{"tag": "vps-diagnostic", "protocol": "blackhole"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	o := attempt(ctx, ports[0], srv.URL, 204)
	if requests.Load() != 1 || o.Outcome != "http" || o.HTTPStatus != 503 {
		t.Fatalf("curl contract escaped: requests=%d observation=%+v", requests.Load(), o)
	}
}

func TestInstalledCoreAdmissionUnexpectedExit(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("explicit native core required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ports, stop, alive, err := startCoreTracked(ctx, binary, []map[string]any{{"tag": "upstream", "protocol": "freedom"}}, map[string]any{"tag": "vps-diagnostic", "protocol": "blackhole"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !alive() {
		t.Fatal("core unavailable before controls")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	for i := 0; i < 2; i++ {
		if o := attempt(context.Background(), ports[0], srv.URL, 204); o.Outcome != "ok" {
			t.Fatal("control", o)
		}
	}
	// Kill only the core; the collection context and suspect attempts continue.
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for alive() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if alive() {
		t.Fatal("unexpected exit not exposed")
	}
	stop()
	for i := 0; i < 3; i++ {
		o := attempt(context.Background(), ports[0], srv.URL, 204)
		if o.CurlCode != 7 || o.Outcome != "local_proxy_unavailable" {
			t.Fatal("local failure looked like destination failure", o)
		}
	}
}
