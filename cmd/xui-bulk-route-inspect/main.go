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
	samples := flag.Int("sample-seconds", 0, "read-only one-second resource samples, maximum 120")
	health := flag.Bool("health", false, "read-only target service and memory metrics")
	flag.Parse()
	if *samples < 0 || *samples > 120 {
		panic("sample-seconds must be 0..120")
	}
	ctx, c := context.WithTimeout(context.Background(), time.Duration(30+*samples)*time.Second)
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
	if *samples > 0 {
		cmd = fmt.Sprintf(`python3 - <<'PYRESOURCE'

import os,time,json,pathlib
root=pathlib.Path('/proc')
ticks=os.sysconf('SC_CLK_TCK')
prev={};last=time.monotonic()
for index in range(%d+1):
 now=time.monotonic();rss=0;cpu=0;procs=0;current={}
 for p in root.iterdir():
  if not p.name.isdigit():continue
  try:
   comm=(p/'comm').read_text().strip()
   if comm!='x-ui' and not comm.startswith('xray'):continue
   fields=(p/'stat').read_text().rsplit(')',1)[1].split()
   used=int(fields[11])+int(fields[12]);key=p.name+':'+fields[19]
   current[key]=used
   if key in prev:cpu+=(used-prev[key])/ticks/max(now-last,.001)*100
   for line in (p/'status').read_text().splitlines():
    if line.startswith('VmRSS:'):rss+=int(line.split()[1])
   procs+=1
  except (FileNotFoundError,ProcessLookupError):continue
 available=next(int(line.split()[1]) for line in (root/'meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))
 cg=pathlib.Path('/sys/fs/cgroup/system.slice/x-ui.service/memory.current')
 group=int(cg.read_text()) if cg.exists() else None
 print(json.dumps(dict(time=time.time(),index=index,rss_kib=rss,cpu_pct=round(cpu,2) if index else None,processes=procs,available_kib=available,cgroup_memory_bytes=group)),flush=True)
 prev=current;last=now
 if index<%d:time.sleep(1)

PYRESOURCE`, *samples, *samples)
	}
	r, e := ssh.RunDetailed(ctx, t, key, cmd)
	fmt.Print(r.Stdout)
	if e != nil {
		fmt.Printf("ERR %v %s\n", e, r.Stderr)
	}
}
