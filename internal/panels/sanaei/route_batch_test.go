package sanaei

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalRouteBatchReadOnlyAndComplete(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.Method != "POST" || r.URL.Path != "/private/panel/api/xray/routeTest" || r.Header.Get("Cookie") != "session=private-cookie" || r.Header.Get("X-CSRF-Token") != "private-csrf" {
			t.Error("request crossed endpoint/auth boundary")
		}
		if e := r.ParseForm(); e != nil {
			t.Error(e)
		}
		fmt.Fprint(w, `{"success":true,"obj":{"matched":true,"outboundTag":"verified-plan"}}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	requests := make([]SessionRequest, 429)
	for i := range requests {
		requests[i] = SessionRequest{Method: "POST", Path: "panel/api/xray/routeTest", ContentType: "application/x-www-form-urlencoded", Body: []byte("network=tcp&port=443&inboundTag=inbound-" + strconv.Itoa(i/33) + "&email=fixture-" + strconv.Itoa(i) + "&domain=example.com")}
	}
	forms, e := routeBatchForms(requests)
	if e != nil {
		t.Fatal(e)
	}
	input, _ := json.Marshal(map[string]any{"port": port, "path": "/private/", "cookie": "session=private-cookie", "csrf": "private-csrf", "deadline": time.Now().Add(8 * time.Second).Unix(), "forms": forms})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", routeBatchScript)
	cmd.Stdin = bytes.NewReader(input)
	raw, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("local proof process: %v %s", e, raw)
	}
	responses, e := parseRouteBatch(string(raw), len(requests))
	if e != nil || count.Load() != 429 || len(responses) != 429 {
		t.Fatal("partial native proof accepted", count.Load(), len(responses), e)
	}
	if strings.Contains(string(raw), "private-cookie") || strings.Contains(string(raw), "private-csrf") {
		t.Fatal("authentication leaked in receipt")
	}
	for _, response := range responses {
		if response.StatusCode != 200 || !bytes.Contains(response.Body, []byte("verified-plan")) {
			t.Fatal("wrong proof result")
		}
	}
	if _, e = parseRouteBatch(string(raw), 430); e == nil {
		t.Fatal("missing result accepted")
	}
	bad := requests[0]
	bad.Path = "panel/api/inbounds/add"
	if _, e = routeBatchForms([]SessionRequest{bad}); e == nil {
		t.Fatal("mutation accepted")
	}
	bad = requests[0]
	bad.Body = []byte("url=http://external.test")
	if _, e = routeBatchForms([]SessionRequest{bad}); e == nil {
		t.Fatal("arbitrary destination accepted")
	}
	if _, e = parseRouteBatch(`{"results":[{"index":1,"status":200,"success":true}]}`, 1); e == nil {
		t.Fatal("unordered response accepted")
	}
}
