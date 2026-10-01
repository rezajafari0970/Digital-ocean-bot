package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultrconsole"
)

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
func start(name string, args ...string) (*exec.Cmd, error) {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Start()
	return c, err
}

func main() {
	if len(os.Args) != 2 {
		panic("account id required")
	}
	accountID := os.Args[1]
	ctx := context.Background()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}
	defer a.Close()
	var proxyID, host, user, ref string
	var port int
	err = a.DB.QueryRowContext(ctx, `SELECT COALESCE(p.id::text,''),COALESCE(p.host,''),COALESCE(p.port,0),COALESCE(p.username,''),COALESCE(p.secret_ref,'') FROM account_proxy_pool ap JOIN proxies p ON p.id=ap.proxy_id WHERE ap.account_id=$1 AND ap.enabled=true ORDER BY ap.priority LIMIT 1`, accountID).Scan(&proxyID, &host, &port, &user, &ref)
	if err != nil {
		panic(err)
	}
	pass, err := a.Container.Secrets.GetProxy(ctx, proxyID, ref)
	if err != nil {
		panic(err)
	}
	defer wipe(pass)
	up := &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", host, port), User: url.UserPassword(user, string(pass))}
	bridge := &vultrconsole.ProxyBridge{Upstream: up}
	localProxy, err := bridge.Start()
	if err != nil {
		panic(err)
	}
	defer bridge.Close()

	profile := filepath.Join("/var/lib/digital-ocean-bot/browser-sessions", accountID)
	if err = os.MkdirAll(profile, 0700); err != nil {
		panic(err)
	}
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		_ = os.Remove(filepath.Join(profile, name))
	}
	xvfb, err := start("Xvfb", ":199", "-screen", "0", "412x915x24", "-nolisten", "tcp")
	if err != nil {
		panic(err)
	}
	defer xvfb.Process.Kill()
	time.Sleep(time.Second)
	vnc, err := start("x11vnc", "-display", ":199", "-localhost", "-forever", "-shared", "-nopw", "-rfbport", "15900")
	if err != nil {
		panic(err)
	}
	defer vnc.Process.Kill()
	web, err := start("websockify", "--web=/usr/share/novnc/", "127.0.0.1:16080", "127.0.0.1:15900")
	if err != nil {
		panic(err)
	}
	defer web.Process.Kill()
	chrome := exec.Command("/usr/bin/google-chrome-stable",
		"--no-sandbox", "--disable-dev-shm-usage", "--no-first-run",
		"--user-data-dir="+profile, "--proxy-server="+localProxy,
		"--window-size=412,915", "--force-device-scale-factor=1", "--touch-events=enabled",
		"--disable-session-crashed-bubble", "--hide-crash-restore-bubble", "--disable-infobars",
		"--user-agent=Mozilla/5.0 (Linux; Android 16; Mobile) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36",
		"--app=https://my.vultr.com/")
	chrome.Env = append(os.Environ(), "DISPLAY=:199")
	chrome.Stdout = os.Stdout
	chrome.Stderr = os.Stderr
	if err = chrome.Start(); err != nil {
		panic(err)
	}
	defer chrome.Process.Kill()
	_, _ = a.DB.ExecContext(ctx, `UPDATE accounts SET console_capacity_status='manual_session_open',console_capacity_detail='interactive console session available' WHERE id=$1`, accountID)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-sig:
	case <-time.After(30 * time.Minute):
	}
}
