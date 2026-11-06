package providerbrowser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Executed only by the disposable CI socket-activated unit, with the production
// Worker and sandbox directives. The wrapper trusts only the fixture's SPKI.
func TestProviderBrowserWorkerProcess(t *testing.T) {
	if os.Getenv("DOB_TEST_BROWSER_WORKER") != "1" {
		return
	}
	fail := func(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(2) }
	if os.Getuid() == 0 {
		fail(errors.New("worker is root"))
	}
	if e := VerifyIsolation(); e != nil {
		fail(e)
	}
	for _, path := range []string{"/etc/dob-browser-host-fixture", "/var/lib/dob-browser-host-fixture", "/run/dob-browser-host-fixture"} {
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			fail(errors.New("host filesystem visible"))
		}
	}
	conn, e := net.FileConn(os.Stdin)
	if e != nil {
		fail(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	p := NewPeer(ctx, conn)
	w := NewWorker(p, "/usr/local/lib/datacenter-bot/browser-fixture-chrome")
	p.Handle = func(ctx context.Context, a Action) (State, error) {
		s, e := w.Handle(ctx, a)
		if e != nil {
			fmt.Fprintf(os.Stderr, "fixture action %s: %v\n", a.Kind, e)
		}
		return s, e
	}
	p.Start()
	<-p.Done()
	p.Wait()
	w.Close()
	os.Exit(0)
}
func TestProviderBrowserNativeSandbox(t *testing.T) {
	if os.Getenv("DOB_RUN_PROVIDER_BROWSER_SANDBOX") != "1" {
		t.Skip("disposable systemd sandbox opt-in")
	}
	var direct, requests, sockets atomic.Int64
	var missingBrowserMetadata atomic.Int64
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1); w.WriteHeader(200) }))
	defer trap.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/" && (r.Header.Get("Sec-CH-UA") == "" || !strings.HasPrefix(r.Header.Get("Accept-Language"), "de-DE")) {
			missingBrowserMetadata.Add(1)
		}
		if r.URL.Path == "/ws" {
			conn, _, _, err := ws.UpgradeHTTP(r, w)
			if err != nil {
				return
			}
			sockets.Add(1)
			go func() {
				defer sockets.Add(-1)
				defer conn.Close()
				for {
					data, op, e := wsutil.ReadClientData(conn)
					if e != nil {
						return
					}
					if wsutil.WriteServerMessage(conn, op, data) != nil {
						return
					}
				}
			}()
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/input" {
			fmt.Fprint(w, nativeInputFixture)
			return
		}
		fmt.Fprintf(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><input id="value"><button onclick="document.title=document.querySelector('input').value">Set title</button><script>
const q=new URLSearchParams(location.search).get('set');if(q){document.cookie='identity='+q+';SameSite=Lax';localStorage.identity=q}
document.title=(document.cookie||'empty')+'|'+(localStorage.identity||'empty')+'|'+navigator.language+'|'+Intl.DateTimeFormat().resolvedOptions().timeZone+'|native-ua:'+Boolean(navigator.userAgentData?.brands?.length);
const socket=new WebSocket('wss://fixture.invalid/ws');socket.onopen=()=>socket.send('ok');socket.onmessage=e=>document.title+='|ws:'+e.data;
fetch('https://unapproved.fixture/').catch(()=>{});fetch(%q).catch(()=>{});try{const p=new RTCPeerConnection({iceServers:[{urls:'stun:1.1.1.1:3478'}]});p.createDataChannel('x');p.createOffer().then(x=>p.setLocalDescription(x));setTimeout(()=>p.close(),1000)}catch{}
</script></body></html>`, trap.URL)
	}))
	defer server.Close()
	hash := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	if b, e := exec.Command("sudo", "systemctl", "set-environment", "DOB_TEST_BROWSER_SPKI="+base64.StdEncoding.EncodeToString(hash[:])).CombinedOutput(); e != nil {
		t.Fatal(string(b), e)
	}
	defer exec.Command("sudo", "systemctl", "unset-environment", "DOB_TEST_BROWSER_SPKI").Run()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var blocked atomic.Int64
	open := func(raw string) (*Peer, State) {
		conn, e := net.Dial("unix", "/run/datacenter-provider-browser.sock")
		if e != nil {
			t.Fatal(e)
		}
		p := NewPeer(ctx, conn)
		p.Dial = func(ctx context.Context, addr string) (net.Conn, error) {
			if addr != "fixture.invalid:443" {
				blocked.Add(1)
				return nil, errors.New("destination blocked")
			}
			var d net.Dialer
			return d.DialContext(ctx, "tcp", server.Listener.Addr().String())
		}
		p.Start()
		t.Cleanup(p.Close)
		s, e := p.Call(ctx, Action{Kind: "start", URL: raw, Timezone: "Europe/Berlin", Locale: "de-DE", Width: 390, Height: 800})
		if e != nil {
			t.Fatal("sandbox start", e)
		}
		return p, s
	}
	first, _ := open("https://fixture.invalid/?set=first")
	second, _ := open("https://fixture.invalid/")
	// The two browser services share the original total hard budget even though
	// a single heavy provider may borrow more of that budget.
	group, err := exec.Command("systemctl", "show", "datacenter-provider-browser.slice", "-p", "ControlGroup", "--value").Output()
	if err != nil {
		t.Fatal("browser resource slice unavailable", err)
	}
	capBytes, err := os.ReadFile("/sys/fs/cgroup" + strings.TrimSpace(string(group)) + "/memory.max")
	if err != nil || strings.TrimSpace(string(capBytes)) != "1342177280" {
		t.Fatal("combined browser memory budget missing", string(capBytes), err)
	}

	frame := func(p *Peer, want string) State {
		deadline := time.Now().Add(15 * time.Second)
		var last State
		for time.Now().Before(deadline) {
			s, e := p.Call(ctx, Action{Kind: "frame"})
			if e != nil {
				t.Fatal(e)
			}
			if s.Title == want {
				if !s.Isolated || len(s.Image) == 0 || s.Width != 390 {
					t.Fatal("invalid frame", s.Isolated, s.Width)
				}
				return s
			}
			last = s
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("fixture did not render: want=%q actual=%q url=%q requests=%d sockets=%d blocked=%d", want, last.Title, last.URL, requests.Load(), sockets.Load(), blocked.Load())
		return State{}
	}
	frame(first, "identity=first|first|de-DE|Europe/Berlin|native-ua:true|ws:ok")
	frame(second, "empty|empty|de-DE|Europe/Berlin|native-ua:true|ws:ok")
	if blocked.Load() < 2 {
		t.Fatal("unapproved destinations did not reach the guarded bridge", blocked.Load())
	}
	if missingBrowserMetadata.Load() != 0 {
		t.Fatal("native Chrome Client Hints or configured language missing", missingBrowserMetadata.Load())
	}
	if requests.Load() < 2 {
		t.Fatal("browser did not reach fixture through bridge")
	}
	if direct.Load() != 0 {
		t.Fatal("browser contacted host loopback directly")
	}
	for _, a := range []Action{{Kind: "tap", X: 40, Y: 18}, {Kind: "text", Text: "typed in isolated browser"}, {Kind: "key", Key: "Tab"}, {Kind: "key", Key: "Enter"}} {
		if _, e := first.Call(ctx, a); e != nil {
			t.Fatal("native input", e)
		}
	}
	rendered := frame(first, "typed in isolated browser")
	if len(rendered.Editable) == 0 || rendered.Editable[0].Width <= 0 || rendered.Editable[0].Height <= 0 {
		t.Fatal("editable geometry missing")
	}

	focused, e := first.Call(ctx, Action{Kind: "tap", X: 40, Y: 18})
	if e != nil || focused.Focused == nil {
		t.Fatal("native focused editing state unavailable", e)
	}
	f := focused.Focused
	if _, e = first.Call(ctx, Action{Kind: "edit", Field: "wrong-node", Expected: f.Value, Text: "wrong"}); e == nil {
		t.Fatal("foreign focused node edit accepted")
	}
	if _, e = first.Call(ctx, Action{Kind: "edit", Field: f.Token, Expected: "stale-value", Text: "wrong"}); e == nil {
		t.Fatal("stale focused value edit accepted")
	}
	edited, e := first.Call(ctx, Action{Kind: "edit", Field: f.Token, Expected: f.Value, Text: "Hello فارسی 👋", SelectionStart: 2, SelectionEnd: 7})
	if e != nil || edited.Focused == nil || edited.Focused.Value != "Hello فارسی 👋" || edited.Focused.Start != 2 || edited.Focused.End != 7 {
		t.Fatalf("native Unicode edit and selection failed: error=%v focused=%+v", e, edited.Focused)
	}
	if _, e = first.Call(ctx, Action{Kind: "edit", Field: f.Token, Expected: edited.Focused.Value, Text: f.Value, SelectionStart: 0, SelectionEnd: 0}); e != nil {
		t.Fatal("restore native input fixture", e)
	}

	resized, e := first.Call(ctx, Action{Kind: "resize", Width: 800, Height: 360, ScaleFactor: 2})
	if e != nil || resized.Width != 800 || resized.Height != 360 || !resized.Isolated {
		t.Fatal("live viewport resize", resized.Width, resized.Height, e)
	}
	imageSize, e := png.DecodeConfig(bytes.NewReader(resized.Image))
	if e != nil || imageSize.Width != 1600 || imageSize.Height != 720 || resized.Format != "png" || resized.ScaleFactor != 2 {
		t.Fatal("resized pixels do not match viewport", imageSize, e)
	}
	if _, e = first.Call(ctx, Action{Kind: "pointer", Phase: "down", Buttons: 1, X: 20, Y: 200}); e != nil {
		t.Fatal(e)
	}
	if _, e = first.Call(ctx, Action{Kind: "pointer", Phase: "up", X: 20, Y: 200}); e != nil {
		t.Fatal(e)
	}
	still, e := first.Call(ctx, Action{Kind: "frame"})
	if e != nil {
		t.Fatal(e)
	}
	unchanged, e := first.Call(ctx, Action{Kind: "frame", FrameID: still.FrameID})
	if e != nil || len(unchanged.Image) != 0 || unchanged.FrameID != still.FrameID {
		t.Fatal("unchanged high-resolution screen was retransmitted", e)
	}
	if _, e = first.Call(ctx, Action{Kind: "resize", Width: 800, Height: 0}); e == nil {
		t.Fatal("invalid resize accepted")
	}
	if _, e = first.Call(ctx, Action{Kind: "resize", Width: 390, Height: 800, Mobile: true, Touch: true, ScaleFactor: 3}); e != nil {
		t.Fatal(e)
	}
	frame(first, "typed in isolated browser")
	for _, phase := range []string{"start", "move", "end"} {
		if _, e = first.Call(ctx, Action{Kind: "touch", Phase: phase, X: 30, Y: 200}); e != nil {
			t.Fatal("native touch", e)
		}
	}
	if _, e = first.Call(ctx, Action{Kind: "pointer", Phase: "execute", X: 20, Y: 20}); e == nil {
		t.Fatal("invalid pointer command accepted")
	}

	if sockets.Load() != 2 {
		t.Fatal("native WebSocket tunnels not active", sockets.Load())
	}
	nativePageInput(t, first)
	first.Close()
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("closed browser remained alive")
	}
	if _, e := first.Call(ctx, Action{Kind: "frame"}); e == nil {
		t.Fatal("closed session served pixels")
	}
	frame(second, "empty|empty|de-DE|Europe/Berlin|native-ua:true|ws:ok")
	deadline := time.Now().Add(3 * time.Second)
	for sockets.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if sockets.Load() != 1 {
		t.Fatal("closed session retained its WebSocket tunnel", sockets.Load())
	}
	// Unsupported executable input must never be forwarded as an eval command.
	if _, e := second.Call(ctx, Action{Kind: "eval", Text: "fetch('http://169.254.169.254/')"}); e == nil {
		t.Fatal("arbitrary CDP/eval accepted")
	}
	if _, e := second.Call(ctx, Action{Kind: "navigate", URL: "file:///etc/passwd"}); e == nil {
		t.Fatal("local file navigation accepted")
	}
	if strings.Contains(frame(second, "empty|empty|de-DE|Europe/Berlin|native-ua:true|ws:ok").URL, "first") {
		t.Fatal("cross-session navigation")
	}
	t.Logf("native isolated browsers: requests=%d, direct=%d, rejected destinations=%d; cookies/storage separate", requests.Load(), direct.Load(), blocked.Load())
}
