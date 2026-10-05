package adminapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residential"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResidentialLineValidation(t *testing.T) {
	for _, line := range []string{"107.151.196.26:3297:user:pa:ss", "[::1]:1080:user:pass", "proxy.example:80::"} {
		x, e := parseResidentialLine(line)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(line, "pa:ss") && x.Password != "pa:ss" {
			t.Fatal("password changed")
		}
	}
	for _, line := range []string{"host:0:u:p", "host:65536:u:p", "https://host:10:u:p", " host:10:u:p", "host:10:u", "::1:10:u:p", "host:10:u:p\n", "host:10::pass", "host:10:user:"} {
		if _, e := parseResidentialLine(line); e == nil {
			t.Fatalf("accepted invalid format %q", line)
		}
	}
}
func residentialCall(handler http.HandlerFunc, body any, role auth.Role) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/", strings.NewReader(string(raw)))
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: role}))
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func TestResidentialBulkAtomicIdempotentCredentialsAndDelete(t *testing.T) {
	db := adminTestDB(t)
	store, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	s := Server{DB: db, Container: app.Container{Secrets: store}}
	in := residentialImportRequest{RequestID: "12345678-1234-4234-8234-123456789012", Proxies: []residentialImportRow{{Name: "ads1", Line: "127.0.0.1:1:user:pass:colon"}, {Name: "Ads2", Line: "127.0.0.1:2:user:second"}}}
	w := residentialCall(s.importResidentialProxies, in, auth.Admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "pass:colon") {
		t.Fatal("credential leak")
	}
	var result struct{ Proxies []struct{ ID, Name string } }
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Proxies) != 2 || result.Proxies[1].Name != "Ads2" {
		t.Fatal("exact name not preserved")
	}
	ids := []string{result.Proxies[0].ID, result.Proxies[1].ID}
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- residentialCall(s.importResidentialProxies, in, auth.Admin).Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatal("idempotent retry", code)
		}
	}
	var n int
	db.QueryRow("SELECT count(*) FROM residential_proxies").Scan(&n)
	if n != 2 {
		t.Fatal("retry duplicated records", n)
	}
	collision := in
	collision.RequestID = "22345678-1234-4234-8234-123456789012"
	collision.Proxies = []residentialImportRow{{Name: "new-before-error", Line: "127.0.0.1:3:u:p"}, {Name: "ADS1", Line: "127.0.0.1:4:u:p"}}
	w = residentialCall(s.importResidentialProxies, collision, auth.Admin)
	if w.Code != 409 {
		t.Fatal("collision", w.Code)
	}
	db.QueryRow("SELECT count(*) FROM residential_proxies").Scan(&n)
	if n != 2 {
		t.Fatal("partial import")
	}
	collision.RequestID = in.RequestID
	collision.Proxies = []residentialImportRow{{Name: "different", Line: "127.0.0.1:3:u:p"}}
	if w = residentialCall(s.importResidentialProxies, collision, auth.Admin); w.Code != 409 {
		t.Fatal("idempotency conflict accepted")
	}
	for _, handler := range []http.HandlerFunc{s.importResidentialProxies, s.exportResidentialProxies, s.deleteResidentialProxies} {
		if w = residentialCall(handler, in, auth.Viewer); w.Code != 403 {
			t.Fatal("unauthorized operation", w.Code)
		}
	}
	w = residentialCall(s.exportResidentialProxies, residentialIDs{IDs: ids}, auth.Admin)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || !strings.Contains(w.Body.String(), "pass:colon") {
		t.Fatal("export failed", w.Code)
	}
	r := httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	s.residentialProxies(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "pass:colon") {
		t.Fatal("list credential leak")
	}
	var ct string
	db.QueryRow("SELECT encode(ciphertext,'escape') FROM residential_proxy_secrets WHERE residential_id=$1", ids[0]).Scan(&ct)
	if strings.Contains(ct, "pass:colon") {
		t.Fatal("plaintext persisted")
	}
	sqlMust(t, db, "INSERT INTO proxies(id,name,type,host,port) VALUES($1,'shared','socks5','127.0.0.1',1)", ids[0])
	w = residentialCall(s.deleteResidentialProxies, residentialIDs{IDs: ids[:1]}, auth.Admin)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	db.QueryRow("SELECT count(*) FROM proxies WHERE id=$1", ids[0]).Scan(&n)
	if n != 1 {
		t.Fatal("shared account proxy deleted")
	}
	db.QueryRow("SELECT count(*) FROM residential_proxy_secrets WHERE residential_id=$1", ids[0]).Scan(&n)
	if n != 0 {
		t.Fatal("orphan secret")
	}
	db.QueryRow("SELECT count(*) FROM residential_proxies").Scan(&n)
	if n != 1 {
		t.Fatal("unselected proxy deleted")
	}
	w = residentialCall(s.deleteResidentialProxies, residentialIDs{IDs: ids}, auth.Admin)
	if w.Code != 200 {
		t.Fatal("delete retry failed")
	}
	db.QueryRow("SELECT count(*) FROM residential_proxies").Scan(&n)
	if n != 0 {
		t.Fatal("delete all incomplete")
	}
}
func TestResidentialHealthDoesNotChangeRoutingRevision(t *testing.T) {
	db := adminTestDB(t)
	store, _ := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	const id = "12345678-1234-4234-8234-123456789013"
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		c, buf, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		defer c.Close()
		fmt.Fprint(buf, "HTTP/1.1 200 Connection established\r\n\r\n")
		buf.Flush()
		req, e := http.ReadRequest(bufio.NewReader(c))
		if e != nil {
			return
		}
		req.Body.Close()
		payload := `{"ip":"203.0.113.10"}`
		fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(payload), payload)

	}))
	defer server.Close()
	host, portText, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	port, _ := strconv.Atoi(portText)
	sqlMust(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag) VALUES($1,'health','http',$2,$3,'residential-ads-health')", id, host, port)
	var before, after int64
	db.QueryRow("SELECT revision FROM residential_routing_control WHERE singleton").Scan(&before)
	for _, bad := range []bool{false, true, false} {
		fail.Store(bad)
		result, e := (residential.Monitor{DB: db, Secrets: store, Endpoint: "http://203.0.113.1/ip"}).Check(context.Background(), id)
		want := "healthy"
		if bad {
			want = "down"
		}
		if e != nil || result.Status != want {
			t.Fatal(result, e)
		}
	}
	db.QueryRow("SELECT revision FROM residential_routing_control WHERE singleton").Scan(&after)
	if before != after {
		t.Fatal("health triggered routing restart intent")
	}
	var count int
	var status string
	db.QueryRow("SELECT check_count,status FROM residential_proxies WHERE proxy_id=$1", id).Scan(&count, &status)
	if count != 3 || status != "healthy" {
		t.Fatal("health observation missing")
	}
}
