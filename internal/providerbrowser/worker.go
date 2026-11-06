package providerbrowser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// VerifyIsolation is a mandatory second fence: systemd can silently omit a
// namespace on unsupported kernels. A worker must not launch in that situation.
func VerifyIsolation() error {
	interfaces, e := net.Interfaces()
	if e != nil || len(interfaces) == 0 {
		return errors.New("network isolation unavailable")
	}
	for _, v := range interfaces {
		if v.Flags&net.FlagLoopback == 0 {
			return errors.New("external interface in browser sandbox")
		}
	}
	c, e := net.DialTimeout("udp4", "1.1.1.1:53", time.Second)
	if e == nil {
		c.Close()
		return errors.New("external route in browser sandbox")
	}
	return nil
}
func ValidURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && len(raw) <= 4096 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && (u.Port() == "" || u.Port() == "443") && !strings.ContainsAny(raw, "\r\n\x00")
}

type Worker struct {
	Peer          *Peer
	Chrome        string
	base          context.Context
	cancel        context.CancelFunc
	current       context.Context
	tabs          map[target.ID]context.Context
	profile       string
	proxy         *http.Server
	width, height int64
	scaleFactor   float64
	mobile, touch bool
	timezone      string
	locale        string
	isolated      bool
}

func NewWorker(p *Peer, chrome string) *Worker {
	return &Worker{Peer: p, Chrome: chrome, tabs: map[target.ID]context.Context{}}
}
func (w *Worker) Close() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.proxy != nil {
		w.proxy.Close()
	}
	if w.profile != "" {
		os.RemoveAll(w.profile)
	}
}
func (w *Worker) start(a Action) error {
	if w.base != nil {
		return errors.New("already started")
	}
	if !ValidURL(a.URL) {
		return errors.New("invalid provider URL")
	}
	if e := VerifyIsolation(); e != nil {
		return e
	}
	if _, e := time.LoadLocation(a.Timezone); e != nil || a.Timezone == "" {
		return errors.New("timezone unavailable")
	}
	w.width, w.height = a.Width, a.Height
	if !ValidViewport(w.width, w.height) {
		return errors.New("invalid viewport")
	}
	scale, err := DisplayScale(a.Width, a.Height, a.ScaleFactor)
	if err != nil {
		return err
	}
	w.scaleFactor, w.mobile, w.touch = scale, a.Mobile, a.Touch
	w.timezone = a.Timezone
	if a.Locale == "" || len(a.Locale) > 24 || strings.ContainsAny(a.Locale, "\r\n, /\\") {
		return errors.New("locale unavailable")
	}
	w.locale = a.Locale
	root, e := os.MkdirTemp("", "provider-browser-")
	if e != nil {
		return e
	}
	w.profile = root
	profile := filepath.Join(root, "profile")
	if e = os.MkdirAll(filepath.Join(profile, "Default"), 0700); e != nil {
		return e
	}
	prefs, _ := json.Marshal(map[string]any{"intl": map[string]string{"accept_languages": w.locale, "selected_languages": w.locale}})
	if e = os.WriteFile(filepath.Join(profile, "Default", "Preferences"), prefs, 0600); e != nil {
		return e
	}
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return e
	}
	w.proxy = &http.Server{Handler: http.HandlerFunc(w.forward), ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 64 << 10}
	go w.proxy.Serve(ln)
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(w.Chrome), chromedp.UserDataDir(filepath.Join(root, "profile")),
		// Use the full Chrome renderer on an isolated virtual display. Do not spoof
		// navigator.webdriver, user-agent brands or security-challenge fingerprints.
		chromedp.Flag("headless", false), chromedp.Flag("hide-scrollbars", false),
		// The enclosing service supplies the mandatory OS sandbox. Chromium cannot
		// create additional namespaces under RestrictNamespaces and NoNewPrivileges.
		chromedp.Flag("no-sandbox", true), chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-quic", true), chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"),
		chromedp.Flag("disable-background-networking", true), chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-extensions", true), chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"), chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"),
		chromedp.Flag("lang", w.locale),
		chromedp.ProxyServer("http://"+ln.Addr().String()), chromedp.WindowSize(int(w.width), int(w.height)),
		chromedp.Env("HOME="+root, "TZ="+w.timezone),
	)
	if os.Getenv("DOB_TEST_BROWSER_WORKER") == "1" {
		options = append(options, chromedp.CombinedOutput(os.Stderr))
	}
	alloc, allocCancel := chromedp.NewExecAllocator(w.Peer.ctx, options...)
	bctx, browserCancel := chromedp.NewContext(alloc)
	var once sync.Once
	w.cancel = func() { once.Do(func() { browserCancel(); allocCancel() }) }
	w.base, w.current = bctx, bctx
	if e = chromedp.Run(bctx); e != nil {
		return e
	}
	w.isolated = true
	if e = chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error {
		b := cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Browser)
		if e := browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDeny).Do(b); e != nil {
			return e
		}
		return browser.ResetPermissions().Do(b)
	})); e != nil {
		return e
	}
	if e = w.configure(bctx); e != nil {
		return e
	}
	w.tabs[chromedp.FromContext(bctx).Target.TargetID] = bctx
	return chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error { _, _, _, _, e := page.Navigate(a.URL).Do(ctx); return e }))
}
func (w *Worker) configure(ctx context.Context) error {
	// Language is set in the real Chrome profile and command line. Leave the
	// native user agent and Client Hints intact; overriding only the UA string
	// suppresses Chrome's structured Client Hint headers.
	return chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(w.width, w.height, w.scaleFactor, w.mobile),
		emulation.SetTouchEmulationEnabled(w.touch || w.mobile).WithMaxTouchPoints(5),
		emulation.SetTimezoneOverride(w.timezone), emulation.SetLocaleOverride().WithLocale(strings.ReplaceAll(w.locale, "-", "_")))
}

func (w *Worker) forward(out http.ResponseWriter, r *http.Request) {
	address := r.Host
	if r.Method != http.MethodConnect {
		if r.URL.Scheme != "http" || r.URL.Host != r.Host || r.URL.User != nil {
			http.Error(out, "blocked", 403)
			return
		}
		if r.URL.Port() == "" {
			address = net.JoinHostPort(r.URL.Hostname(), "80")
		}
	}
	host, port, e := net.SplitHostPort(address)
	if e != nil || host == "" || (port != "443" && port != "80") {
		http.Error(out, "blocked", 403)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	conn, e := w.Peer.Open(ctx, address)
	if e != nil {
		http.Error(out, "proxy unavailable", 502)
		return
	}
	defer conn.Close()
	hijack, ok := out.(http.Hijacker)
	if !ok {
		http.Error(out, "unsupported", 500)
		return
	}
	client, buf, e := hijack.Hijack()
	if e != nil {
		return
	}
	defer client.Close()
	if r.Method == http.MethodConnect {
		if _, e = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); e != nil {
			return
		}
	} else {
		r.RequestURI = ""
		r.Header.Del("Proxy-Authorization")
		r.Header.Del("Proxy-Connection")
		if e = r.Write(conn); e != nil {
			return
		}
	}
	// Flush any bytes already read by net/http (CONNECT TLS or WebSocket data).
	if buf.Reader.Buffered() > 0 {
		if _, e = io.CopyN(conn, buf, int64(buf.Reader.Buffered())); e != nil {
			return
		}
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(conn, client); conn.Close(); close(done) }()
	_, _ = io.Copy(client, conn)
	client.Close()
	conn.Close()
	<-done
}
func (w *Worker) Handle(ctx context.Context, a Action) (State, error) {
	if a.Kind == "start" {
		if e := w.start(a); e != nil {
			w.Close()
			return State{}, e
		}
	} else {
		if w.current == nil {
			return State{}, errors.New("not started")
		}
		var actions []chromedp.Action
		switch a.Kind {
		case "frame":
		case "resize":
			scale, err := DisplayScale(a.Width, a.Height, a.ScaleFactor)
			if err != nil {
				return State{}, err
			}
			if err = w.run(ctx, emulation.SetDeviceMetricsOverride(a.Width, a.Height, scale, a.Mobile), emulation.SetTouchEmulationEnabled(a.Touch || a.Mobile).WithMaxTouchPoints(5)); err != nil {
				return State{}, err
			}
			w.width, w.height, w.scaleFactor, w.mobile, w.touch = a.Width, a.Height, scale, a.Mobile, a.Touch
		case "pointer":
			if !w.coordinates(a) || (a.Buttons != 0 && a.Buttons != 1) {
				return State{}, errors.New("invalid pointer")
			}
			typ := input.MouseMoved
			switch a.Phase {
			case "down":
				typ = input.MousePressed
			case "up":
				typ = input.MouseReleased
			case "move":
			default:
				return State{}, errors.New("invalid pointer phase")
			}
			event := input.DispatchMouseEvent(typ, a.X, a.Y).WithButtons(a.Buttons)
			if a.Phase != "move" {
				event = event.WithButton(input.Left).WithClickCount(1)
			}
			actions = append(actions, event)
		case "touch":
			if !w.coordinates(a) || !(w.mobile || w.touch) {
				return State{}, errors.New("invalid touch")
			}
			points := []*input.TouchPoint{{X: a.X, Y: a.Y, ID: 1, RadiusX: 1, RadiusY: 1, Force: 1}}
			typ := input.TouchStart
			switch a.Phase {
			case "start":
			case "move":
				typ = input.TouchMove
			case "end":
				typ = input.TouchEnd
				points = nil
			case "cancel":
				typ = input.TouchCancel
				points = nil
			default:
				return State{}, errors.New("invalid touch phase")
			}
			actions = append(actions, input.DispatchTouchEvent(typ, points))
		case "navigate":
			if !ValidURL(a.URL) {
				return State{}, errors.New("invalid URL")
			}
			actions = append(actions, chromedp.ActionFunc(func(c context.Context) error { _, _, _, _, e := page.Navigate(a.URL).Do(c); return e }))
		case "reload":
			actions = append(actions, page.Reload())
		case "back", "forward":
			actions = append(actions, chromedp.ActionFunc(func(c context.Context) error {
				index, entries, e := page.GetNavigationHistory().Do(c)
				if e != nil {
					return e
				}
				step := int64(-1)
				if a.Kind == "forward" {
					step = 1
				}
				index += step
				if index < 0 || index >= int64(len(entries)) {
					return nil
				}
				return page.NavigateToHistoryEntry(entries[index].ID).Do(c)
			}))
		case "tap":
			if !w.coordinates(a) {
				return State{}, errors.New("invalid position")
			}
			actions = append(actions, chromedp.MouseClickXY(a.X, a.Y))
		case "scroll":
			if !w.coordinates(a) || math.IsNaN(a.Delta) || math.Abs(a.Delta) > 1500 {
				return State{}, errors.New("invalid scroll")
			}
			actions = append(actions, input.DispatchMouseEvent(input.MouseWheel, a.X, a.Y).WithDeltaY(a.Delta))
		case "text":
			if len(a.Text) > 8192 {
				return State{}, errors.New("text too long")
			}
			actions = append(actions, input.InsertText(a.Text))
		case "key":
			keys := map[string]string{"Enter": kb.Enter, "Tab": kb.Tab, "Backspace": kb.Backspace, "Delete": kb.Delete, "Escape": kb.Escape, "ArrowUp": kb.ArrowUp, "ArrowDown": kb.ArrowDown, "ArrowLeft": kb.ArrowLeft, "ArrowRight": kb.ArrowRight, "Home": kb.Home, "End": kb.End, "SelectAll": kb.Control + "a"}
			key, ok := keys[a.Key]
			if !ok {
				return State{}, errors.New("unsupported key")
			}
			actions = append(actions, chromedp.KeyEvent(key))
		case "tab":
			list, e := chromedp.Targets(w.base)
			if e != nil {
				return State{}, e
			}
			found := false
			for _, t := range list {
				if string(t.TargetID) == a.Target && t.Type == "page" {
					found = true
				}
			}
			if !found {
				return State{}, errors.New("tab unavailable")
			}
			id := target.ID(a.Target)
			c := w.tabs[id]
			if c == nil {
				c, _ = chromedp.NewContext(w.base, chromedp.WithTargetID(id))
				if e = w.configure(c); e != nil {
					return State{}, e
				}
				w.tabs[id] = c
			}
			if e = w.configure(c); e != nil {
				return State{}, e
			}
			w.current = c
		case "newtab":
			list, e := chromedp.Targets(w.base)
			if e != nil || len(list) >= 8 {
				return State{}, errors.New("tab limit")
			}
			c, _ := chromedp.NewContext(w.base)
			if e = w.configure(c); e != nil {
				return State{}, e
			}
			w.tabs[chromedp.FromContext(c).Target.TargetID] = c
			w.current = c
		default:
			return State{}, errors.New("unsupported action")
		}
		if len(actions) > 0 {
			if e := w.run(ctx, actions...); e != nil {
				return State{}, e
			}
		}
	}
	frame, err := w.frame(ctx)
	if err == nil && a.Kind == "frame" && a.FrameID != "" && a.FrameID == frame.FrameID {
		frame.Image = nil
	}
	return frame, err
}
func (w *Worker) coordinates(a Action) bool {
	return !math.IsNaN(a.X) && !math.IsNaN(a.Y) && a.X >= 0 && a.Y >= 0 && a.X < float64(w.width) && a.Y < float64(w.height)
}
func (w *Worker) run(parent context.Context, actions ...chromedp.Action) error {
	ctx, cancel := context.WithCancel(w.current)
	defer cancel()
	stop := context.AfterFunc(parent, cancel)
	defer stop()
	return chromedp.Run(ctx, actions...)
}
func (w *Worker) frame(ctx context.Context) (State, error) {
	s := State{Width: w.width, Height: w.height, Isolated: w.isolated, Format: "png", ScaleFactor: w.scaleFactor, Mobile: w.mobile}
	e := w.run(ctx, chromedp.Location(&s.URL), chromedp.Title(&s.Title), chromedp.ActionFunc(func(c context.Context) error {
		var e error
		s.Image, e = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).WithCaptureBeyondViewport(false).Do(c)
		// Keep pathological photo/noise frames bounded; normal text/UI frames are lossless.
		if e == nil && len(s.Image) > 8<<20 {
			s.Format = "jpeg"
			s.Image, e = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(95).WithCaptureBeyondViewport(false).Do(c)
		}
		return e
	}))
	if e != nil {
		return State{}, e
	}
	// Fixed read-only geometry query; no user script, field value, cookie or DOM is returned.
	// Cross-origin frames simply use the explicit keyboard control.
	_ = w.run(ctx, chromedp.Evaluate(`(() => {
      const out=[];
      function scan(doc,ox,oy){
       for(const el of doc.querySelectorAll('input,textarea,[contenteditable="true"],iframe')){
        if(out.length>=64)break;
        const r=el.getBoundingClientRect(),x=r.x+ox,y=r.y+oy;
        if(r.width<=0||r.height<=0||x+r.width<0||y+r.height<0||x>innerWidth||y>innerHeight)continue;
        if(el.tagName==='IFRAME'){try{if(el.contentDocument)scan(el.contentDocument,x,y)}catch{}continue}
        if(el.disabled||el.readOnly)continue;
        if(el.tagName==='INPUT'&&!['text','password','email','search','tel','url','number'].includes(el.type))continue;
        const style=doc.defaultView.getComputedStyle(el);if(style.visibility==='hidden'||style.display==='none')continue;
        out.push({x,y,width:r.width,height:r.height});
       }
      }
      scan(document,0,0);return out;
    })()`, &s.Editable))
	sum := sha256.Sum256(s.Image)
	s.FrameID = hex.EncodeToString(sum[:])
	s.Target = string(chromedp.FromContext(w.current).Target.TargetID)
	ts, e := chromedp.Targets(w.base)
	if e != nil {
		return State{}, e
	}
	for _, t := range ts {
		if t.Type == "page" {
			s.Tabs = append(s.Tabs, Tab{ID: string(t.TargetID), Title: t.Title, URL: t.URL})
		}
	}
	return s, nil
}

// Kept deliberately narrow; the panel sends pixels/input, never CDP commands,
// arbitrary scripts, host paths, command lines, credentials or browser profiles.

// DisplayScale bounds physical raster work while preserving high-DPI mobile text.
func DisplayScale(width, height int64, requested float64) (float64, error) {
	if !ValidViewport(width, height) || math.IsNaN(requested) || math.IsInf(requested, 0) || requested < 0 || requested > 3 {
		return 0, errors.New("invalid display")
	}
	if requested == 0 {
		requested = 1
	}
	if requested < 1 {
		return 0, errors.New("invalid display scale")
	}
	return math.Min(requested, math.Sqrt(4000000/float64(width*height))), nil
}
