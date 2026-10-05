package residentialsync

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func TestPoolPlanScopeFingerprintAndUDP(t *testing.T) {
	p := routePolicy{PoolEnabled: true, Harden: true, AdsOnly: true, Residential: true, Configured: 3, Proxies: []rp{{Type: "http", Host: "127.0.0.1", Port: 1, Tag: "residential-ads-http"}, {Type: "socks5", Host: "127.0.0.1", Port: 2, Tag: "residential-ads-socks"}}}
	base := map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}
	first, e := buildSettings(base, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	second, e := buildSettings(first, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	if settingsHash(first) != settingsHash(second) {
		t.Fatal("pool plan not idempotent")
	}
	routing := first["routing"].(map[string]any)
	ads := 0
	for _, v := range routing["rules"].([]any) {
		m := v.(map[string]any)
		tag, _ := m["ruleTag"].(string)
		if strings.HasPrefix(tag, "dob-route-residential-ads-") {
			ads++
			domains := m["domain"].([]string)
			if strings.Join(domains, ",") != "geosite:google@ads,domain:browserleaks.com" {
				t.Fatal(domains)
			}
			if m["outboundTag"] != nil {
				t.Fatal("pool bypass")
			}
		}
	}
	if ads != 2 {
		t.Fatal("missing TCP/UDP split")
	}
	for _, v := range routing["balancers"].([]any) {
		m := v.(map[string]any)
		if m["fallbackTag"] != tagged(first, blockedTag) {
			t.Fatal("unsafe fallback")
		}
		tag := m["tag"].(string)
		if tag == poolPrefix+"udp-all" || tag == poolPrefix+"udp-fast" {
			ss := m["selector"].([]string)
			if len(ss) != 1 || ss[0] != tagged(first, "residential-ads-socks") {
				t.Fatal("HTTP selected for UDP", ss)
			}
		}
	}
	legacy := p
	legacy.PoolEnabled = false
	rollback, e := buildSettings(first, nil, []string{"in"}, legacy)
	if e != nil {
		t.Fatal(e)
	}
	if rollback["burstObservatory"] != nil || rollback["routing"].(map[string]any)["balancers"] != nil {
		t.Fatal("rollback kept obsolete pool references")
	}
	p.Proxies = nil
	empty, e := buildSettings(first, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	if empty["burstObservatory"] != nil {
		t.Fatal("removed pool still probing")
	}
}

type poolFixture struct {
	listener net.Listener
	udp      *net.UDPConn
	down     atomic.Bool
	checks   atomic.Int64
	failed   atomic.Int64
	tcpHits  atomic.Int64
	udpHits  atomic.Int64
	delay    time.Duration
	label    string
	gate     <-chan struct{}
}

func newPoolFixture(t *testing.T, label string, delay time.Duration, gate <-chan struct{}) *poolFixture {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	u, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	f := &poolFixture{listener: l, udp: u, delay: delay, label: label, gate: gate}
	t.Cleanup(func() { l.Close(); u.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go f.serve(c)
		}
	}()
	go func() {
		buf := make([]byte, 8192)
		for {
			n, addr, e := u.ReadFromUDP(buf)
			if e != nil {
				return
			}
			if f.down.Load() {
				continue
			}
			off := socksAddressSize(buf[3:n]) + 3
			if off <= 3 || off > n {
				continue
			}
			f.udpHits.Add(1)
			reply := append(append([]byte{}, buf[:off]...), []byte(f.label)...)
			u.WriteToUDP(reply, addr)
		}
	}()
	return f
}
func socksAddressSize(b []byte) int {
	if len(b) < 1 {
		return 0
	}
	switch b[0] {
	case 1:
		return 7
	case 4:
		return 19
	case 3:
		if len(b) > 1 {
			return 4 + int(b[1])
		}
	}
	return 0
}
func (f *poolFixture) serve(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(15 * time.Second))
	h := make([]byte, 2)
	if _, e := io.ReadFull(c, h); e != nil {
		return
	}
	methods := make([]byte, int(h[1]))
	if _, e := io.ReadFull(c, methods); e != nil {
		return
	}
	c.Write([]byte{5, 0})
	head := make([]byte, 4)
	if _, e := io.ReadFull(c, head); e != nil {
		return
	}
	size := 0
	switch head[3] {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		b := make([]byte, 1)
		if _, e := io.ReadFull(c, b); e != nil {
			return
		}
		size = int(b[0])
	default:
		return
	}
	if _, e := io.CopyN(io.Discard, c, int64(size+2)); e != nil {
		return
	}
	if head[1] == 3 {
		port := f.udp.LocalAddr().(*net.UDPAddr).Port
		c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(port >> 8), byte(port)})
		io.Copy(io.Discard, c)
		return
	}
	c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	req, e := http.ReadRequest(bufio.NewReader(c))
	if e != nil {
		return
	}
	defer req.Body.Close()
	if req.URL.Path == "/probe" {
		if f.gate != nil {
			<-f.gate
		}
		time.Sleep(f.delay)
		if f.down.Load() {
			f.failed.Add(1)
			return
		}
		fmt.Fprint(c, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
		f.checks.Add(1)
		return
	}
	if f.down.Load() {
		return
	}
	f.tcpHits.Add(1)
	fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(f.label), f.label)
}
func poolRequest(address string) (string, error) {
	d, e := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: time.Second})
	if e != nil {
		return "", e
	}
	c, e := d.Dial("tcp", "adservice.google.com:80")
	if e != nil {
		return "", e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	fmt.Fprint(c, "GET /traffic HTTP/1.1\r\nHost: adservice.google.com\r\nConnection: close\r\n\r\n")
	resp, e := http.ReadResponse(bufio.NewReader(c), nil)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	return string(b), e
}
func poolUDPRequest(address string) (string, error) {
	c, e := net.DialTimeout("tcp", address, time.Second)
	if e != nil {
		return "", e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	c.Write([]byte{5, 1, 0})
	b := make([]byte, 2)
	if _, e = io.ReadFull(c, b); e != nil {
		return "", e
	}
	c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	bound := make([]byte, 10)
	if _, e = io.ReadFull(c, bound); e != nil {
		return "", e
	}
	u, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(binary.BigEndian.Uint16(bound[8:]))})
	if e != nil {
		return "", e
	}
	defer u.Close()
	u.SetDeadline(time.Now().Add(time.Second))
	domain := "adservice.google.com"
	packet := append([]byte{0, 0, 0, 3, byte(len(domain))}, domain...)
	packet = append(packet, 0, 80, 1)
	u.Write(packet)
	buf := make([]byte, 8192)
	n, e := u.Read(buf)
	if e != nil {
		return "", e
	}
	off := 3 + socksAddressSize(buf[3:n])
	if off > n {
		return "", fmt.Errorf("invalid UDP response")
	}
	return string(buf[off:n]), nil
}
func poolWait(t *testing.T, why string, fn func() bool) {
	t.Helper()
	until := time.Now().Add(26 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			time.Sleep(80 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timeout:", why)
}
func TestInstalledCorePoolFaultDistributionRecovery(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("installed core required")
	}
	gate := make(chan struct{})
	fast := newPoolFixture(t, "fast", 10*time.Millisecond, gate)
	slow := newPoolFixture(t, "slow", 300*time.Millisecond, gate)
	dead := newPoolFixture(t, "dead", 10*time.Millisecond, gate)
	dead.down.Store(true)
	p := routePolicy{PoolEnabled: true, Harden: true, AdsOnly: true, Residential: true, Configured: 3}
	for _, f := range []*poolFixture{fast, slow, dead} {
		p.Proxies = append(p.Proxies, rp{Type: "socks5", Host: "127.0.0.1", Port: f.listener.Addr().(*net.TCPAddr).Port, Tag: "residential-ads-" + f.label})
	}
	setting, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}, nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	setting["burstObservatory"].(map[string]any)["pingConfig"].(map[string]any)["destination"] = "http://127.0.0.1:1/probe"
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	setting["log"] = map[string]any{"loglevel": "none"}
	setting["inbounds"] = []any{map[string]any{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}}
	file := filepath.Join(t.TempDir(), "pool.json")
	raw, _ := json.Marshal(setting)
	os.WriteFile(file, raw, 0600)
	if out, e := exec.Command(binaryPath, "run", "-test", "-config", file).CombinedOutput(); e != nil {
		t.Fatalf("core config rejected: %v %s", e, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath, "run", "-config", file)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cancel(); cmd.Wait() }()
	poolWait(t, "core ready", func() bool {
		c, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e == nil
	})
	if _, e = poolRequest(address); e == nil {
		close(gate)
		t.Fatal("untested proxy used")
	}
	close(gate)
	poolWait(t, "initial health", func() bool { return fast.checks.Load() > 0 && slow.checks.Load() > 0 && dead.failed.Load() > 0 })
	sample := func(n int, udp bool) map[string]int {
		counts := map[string]int{}
		for i := 0; i < n; i++ {
			var value string
			var err error
			if udp {
				value, err = poolUDPRequest(address)
			} else {
				value, err = poolRequest(address)
			}
			if err != nil {
				t.Fatal("healthy request failed", err)
			}
			counts[value]++
		}
		return counts
	}
	for _, udp := range []bool{false, true} {
		counts := sample(120, udp)
		if counts["fast"] < 80 || counts["slow"] < 2 || counts["dead"] != 0 {
			t.Fatal("bias/distribution failure", udp, counts)
		}
		t.Log("healthy distribution UDP", udp, counts)
	}
	failed := fast.failed.Load()
	fast.down.Store(true)
	poolWait(t, "failure exclusion", func() bool { return fast.failed.Load() > failed })
	counts := sample(35, false)
	if counts["slow"] != 35 {
		t.Fatal("failed proxy used", counts)
	}
	successes := fast.checks.Load()
	fast.down.Store(false)
	poolWait(t, "recovery", func() bool { return fast.checks.Load() > successes })
	counts = sample(100, false)
	if counts["fast"] < 65 || counts["slow"] == 0 {
		t.Fatal("recovery or exploration missing", counts)
	}
	t.Log("recovery distribution", counts)
	f1, f2 := fast.failed.Load(), slow.failed.Load()
	fast.down.Store(true)
	slow.down.Store(true)
	poolWait(t, "all unavailable", func() bool { return fast.failed.Load() > f1 && slow.failed.Load() > f2 })
	if _, e = poolRequest(address); e == nil {
		t.Fatal("all-down pool escaped")
	}
	if dead.tcpHits.Load() != 0 || dead.udpHits.Load() != 0 {
		t.Fatal("known-dead endpoint used")
	}
	t.Log("unknown exclusion, TCP/UDP distribution, failure withdrawal, recovery and all-down blocking verified")
}

func TestPoolPreservesSanaeiAPIRuleBeforeFingerprint(t *testing.T) {
	p := routePolicy{PoolEnabled: true, Harden: true, AdsOnly: true, Residential: true, Configured: 1, Proxies: []rp{{Type: "socks5", Host: "127.0.0.1", Port: 1, Tag: "residential-ads-test"}}}
	first, e := buildSettings(baseSettings(), nil, []string{"in"}, p)
	if e != nil {
		t.Fatal(e)
	}
	rules := first["routing"].(map[string]any)["rules"].([]any)
	if rules[0].(map[string]any)["outboundTag"] != "api" {
		t.Fatal("Sanaei would rewrite API rule ordering")
	}
	if !strings.HasPrefix(rules[1].(map[string]any)["ruleTag"].(string), poolPrefix) {
		t.Fatal("pool stage must follow API")
	}
	// Model the panel's EnsureStatsRouting normalizer, then re-read/rebuild.
	raw, _ := json.Marshal(first)
	observed, _ := decodeObject(raw)
	rs := observed["routing"].(map[string]any)["rules"].([]any)
	for i, v := range rs {
		if v.(map[string]any)["outboundTag"] == "api" {
			rs = append([]any{v}, append(rs[:i], rs[i+1:]...)...)
			break
		}
	}
	observed["routing"].(map[string]any)["rules"] = rs
	if settingsHash(first) != settingsHash(observed) {
		t.Fatal("save-time normalization changed fingerprint")
	}
	next, e := buildSettings(observed, nil, []string{"in"}, p)
	if e != nil || settingsHash(next) != settingsHash(first) {
		t.Fatal("readback/reconcile not stable", e)
	}
}
