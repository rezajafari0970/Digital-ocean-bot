package main

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providerbrowser"
)

func main() {
	// inetd-style socket activation; no public listening port or command API.
	if providerbrowser.VerifyIsolation() != nil {
		os.Exit(1)
	}
	conn, err := net.FileConn(os.Stdin)
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	p := providerbrowser.NewPeer(ctx, conn)
	w := providerbrowser.NewWorker(p, "/usr/local/lib/datacenter-bot/browser/chrome-headless-shell-linux64/chrome-headless-shell")
	defer w.Close()
	p.Handle = w.Handle
	p.Start()
	<-p.Done()
	p.Wait()
}
