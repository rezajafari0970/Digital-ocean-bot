package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/serverprotection"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "use run, configure, status, cleanup or version")
		os.Exit(2)
	}
	var e error
	switch os.Args[1] {
	case "run":
		e = serverprotection.RunAgent(ctx)
	case "configure":
		e = serverprotection.Configure(os.Stdin)
	case "cleanup":
		e = serverprotection.Cleanup(ctx)
	case "status":
		var s serverprotection.Status
		s, e = serverprotection.ReadStatus()
		if e == nil {
			e = json.NewEncoder(os.Stdout).Encode(s)
		}
	case "wait-status":
		if len(os.Args) != 3 {
			e = fmt.Errorf("revision required")
			break
		}
		var rev int64
		rev, e = strconv.ParseInt(os.Args[2], 10, 64)
		if e != nil {
			break
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			var s serverprotection.Status
			s, e = serverprotection.ReadStatus()
			if e == nil && s.Revision == rev && s.SampleAgeMS < 5000 {
				e = json.NewEncoder(os.Stdout).Encode(s)
				break
			}
			if time.Now().After(deadline) {
				e = fmt.Errorf("guardian current revision not observed")
				break
			}
			select {
			case <-ctx.Done():
				e = ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
			if ctx.Err() != nil {
				break
			}
		}
	case "version":
		fmt.Println(serverprotection.Version)
	default:
		e = fmt.Errorf("unknown operation")
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
