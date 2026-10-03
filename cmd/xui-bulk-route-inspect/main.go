package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"time"
)

func main() {
	health := flag.Bool("health", false, "read-only target service and memory metrics")
	flag.Parse()
	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()
	a, e := app.Bootstrap(ctx)
	if e != nil {
		panic(e)
	}
	defer a.Close()
	var acc, did, host, user, keyref string
	e = a.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','') FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1`, "11f21262-1b20-4080-8ffc-7528a01679a9").Scan(&acc, &did, &host, &user, &keyref)
	if e != nil {
		panic(e)
	}
	key, e := a.Container.Secrets.Get(ctx, acc, keyref)
	if e != nil {
		panic(e)
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: a.DB}}
	t := provisioning.Target{AccountID: acc, DropletID: did, Host: host, Port: 22, User: user, KeySecretRef: keyref}
	cmd := `for f in /usr/local/x-ui/x-ui /usr/local/x-ui/bin/x-ui; do if [ -f "$f" ]; then strings "$f" 2>/dev/null | grep -A85 -B2 '"/panel/api/clients/bulkCreate"' | head -95 || true; fi; done`
	if *health {
		cmd = `systemctl is-active x-ui; ps -C x-ui -C xray -o comm=,rss=,%cpu=; free -m`
	}
	r, e := ssh.RunDetailed(ctx, t, key, cmd)
	fmt.Print(r.Stdout)
	if e != nil {
		fmt.Printf("ERR %v %s\n", e, r.Stderr)
	}
}
