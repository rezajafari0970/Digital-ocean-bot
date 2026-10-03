package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"time"
)

func main() {
	configTest := flag.Bool("routing-config-test", false, "validate synthetic routing config with installed Xray; no service changes")
	routeProbe := flag.Bool("route-probe", false, "read-only running-core route selection")
	xrayShape := flag.Bool("xray-shape", false, "read-only routing shape; no credentials")
	routingContract := flag.Bool("routing-contract", false, "read-only embedded routing and inbound delete contract")
	lifecycleInventory := flag.Bool("lifecycle-inventory", false, "fresh consistent v3 policy counts only; no client identities")
	globalClients := flag.Bool("global-client-shape", false, "read-only global list response keys and sizes; no client values")
	inventory := flag.Bool("inventory", false, "read-only fresh Sanaei inbound summary")
	panel := flag.String("panel", "11f21262-1b20-4080-8ffc-7528a01679a9", "exact panel for read-only inspection")
	samples := flag.Int("sample-seconds", 0, "read-only one-second resource samples, maximum 120")
	bulkDelete := flag.Bool("bulk-delete-contract", false, "read-only installed bulkDel contract")
	clientRoutes := flag.Bool("client-routes", false, "read-only embedded v3 client route inventory")
	health := flag.Bool("health", false, "read-only target service and memory metrics")
	flag.Parse()
	if *samples < 0 || *samples > 120 {
		panic("sample-seconds must be 0..120")
	}
	ctx, c := context.WithTimeout(context.Background(), time.Duration(45+*samples)*time.Second)
	defer c()
	a, e := app.Bootstrap(ctx)
	if e != nil {
		panic(e)
	}
	defer a.Close()
	if *inventory || *globalClients || *lifecycleInventory || *xrayShape || *routeProbe {
		manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: time.Second}
		rt, e := manager.Acquire(ctx, *panel)
		if e != nil {
			panic(e)
		}
		if *routeProbe {
			resp, err := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/routeTest", ContentType: "application/x-www-form-urlencoded", Body: []byte("ip=1.1.1.1&port=443&network=tcp&inboundTag=in-443-tcp&email=dob-route-verification"), TimeoutSeconds: 10})
			if err != nil {
				panic(err)
			}
			fmt.Printf("status=%d body=%s\n", resp.StatusCode, resp.Body)
			return
		}
		if *xrayShape {
			resp, err := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/", TimeoutSeconds: 10})
			if err != nil {
				panic(err)
			}
			var top struct {
				Success bool
				Obj     json.RawMessage
			}
			if err = json.Unmarshal(resp.Body, &top); err != nil || !top.Success {
				panic("xray read failed")
			}
			var text string
			body := top.Obj
			if json.Unmarshal(body, &text) == nil {
				body = []byte(text)
			}
			var obj map[string]json.RawMessage
			if err = json.Unmarshal(body, &obj); err != nil {
				panic(err)
			}
			for k := range obj {
				fmt.Println("xray_envelope_key=" + k)
			}
			var x map[string]any
			if err = json.Unmarshal(obj["xraySetting"], &x); err != nil {
				panic(err)
			}
			for k := range x {
				fmt.Println("setting_key=" + k)
			}
			for _, v := range x["outbounds"].([]any) {
				m := v.(map[string]any)
				fmt.Printf("outbound tag=%v protocol=%v\n", m["tag"], m["protocol"])
			}
			if routing, ok := x["routing"].(map[string]any); ok {
				for _, v := range routing["rules"].([]any) {
					m := v.(map[string]any)
					out := map[string]any{}
					for _, k := range []string{"type", "ruleTag", "inboundTag", "outboundTag", "network"} {
						if x, ok := m[k]; ok {
							out[k] = x
						}
					}
					raw, _ := json.Marshal(out)
					fmt.Println(string(raw))
				}
			}
			for _, path := range []string{"panel/api/server/getConfigJson", "panel/api/server/status"} {
				response, err := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "GET", Path: path, TimeoutSeconds: 10})
				if err != nil {
					panic(err)
				}
				var value map[string]json.RawMessage
				json.Unmarshal(response.Body, &value)
				fmt.Printf("api=%s status=%d bytes=%d\n", path, response.StatusCode, len(response.Body))
				for k := range value {
					fmt.Printf("root_key=%s\n", k)
				}
				if body, ok := value["obj"]; ok {
					var str string
					if json.Unmarshal(body, &str) == nil {
						body = []byte(str)
					}
					var inner map[string]json.RawMessage
					json.Unmarshal(body, &inner)
					for k := range inner {
						fmt.Printf("obj_key=%s\n", k)
					}
					if x, ok := inner["xray"]; ok {
						var xr map[string]json.RawMessage
						json.Unmarshal(x, &xr)
						for _, k := range []string{"state", "version"} {
							if v, ok := xr[k]; ok {
								fmt.Printf("xray_%s=%s\n", k, v)
							}
						}
					}
				}
			}
			raws, err := rt.Session.Snapshot(ctx)
			if err != nil {
				panic(err)
			}
			for _, raw := range raws {
				var m map[string]any
				json.Unmarshal(raw, &m)
				fmt.Printf("inbound id=%v tag=%v port=%v\n", m["id"], m["tag"], m["port"])
			}
			return
		}
		if *lifecycleInventory {
			obs, _, e := clientops.LifecycleInventory(ctx, rt, 1)
			if e != nil {
				panic(e)
			}
			rows, e := a.DB.QueryContext(ctx, `SELECT o.client_id,o.email FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=1 AND o.state IN ('PLANNED','ACTIVE','DELETE_PENDING')`, *panel)
			if e != nil {
				panic(e)
			}
			owned := map[string]string{}
			for rows.Next() {
				var id, email string
				if e = rows.Scan(&id, &email); e != nil {
					rows.Close()
					panic(e)
				}
				owned[id] = email
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				panic(e)
			}
			manual := map[string]sanaei.Client{}
			active := 0
			for id, c := range obs {
				if email, ok := owned[id]; ok {
					if email != c.Client.Email {
						panic("ownership mismatch")
					}
				} else {
					manual[id] = c.Client
				}
				reason, e := c.InactiveReason(time.Now())
				if e != nil {
					panic(e)
				}
				if reason == "" {
					active++
				}
			}
			baseline, _ := json.Marshal(manual)
			enabled := 0
			quotas := map[int64]int{}
			limits := map[int]int{}
			for _, c := range obs {
				if c.Client.Enable {
					enabled++
				}
				quotas[c.Client.TotalGB]++
				limits[c.Client.LimitHWID]++
			}
			b, _ := json.Marshal(map[string]any{"clients": len(obs), "active": active, "non_owned": len(manual), "non_owned_sha256": fmt.Sprintf("%x", sha256.Sum256(baseline)), "enabled": enabled, "quota_counts": quotas, "hwid_counts": limits})
			fmt.Println(string(b))
			return
		}
		if *globalClients {
			res, e := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "GET", Path: "panel/api/clients/list", TimeoutSeconds: 30})
			if e != nil {
				panic(e)
			}
			fmt.Printf("status=%d body_bytes=%d\n", res.StatusCode, len(res.Body))
			var v map[string]any
			if e = json.Unmarshal(res.Body, &v); e != nil {
				panic(e)
			}
			for k, value := range v {
				if k != "obj" {
					fmt.Printf("envelope_key=%s type=%T\n", k, value)
				}
			}
			switch obj := v["obj"].(type) {
			case []any:
				fmt.Printf("obj_array_count=%d\n", len(obj))
				if len(obj) > 0 {
					if m, ok := obj[0].(map[string]any); ok {
						for k, v := range m {
							fmt.Printf("client_key=%s type=%T\n", k, v)
							if k == "traffic" {
								if traffic, ok := v.(map[string]any); ok {
									for tk, tv := range traffic {
										fmt.Printf("traffic_key=%s type=%T\n", tk, tv)
									}
								}
							}

						}
					}
				}
			case map[string]any:
				for k, v := range obj {
					fmt.Printf("obj_key=%s type=%T\n", k, v)
				}
			default:
				fmt.Printf("obj_type=%T\n", obj)
			}
			return
		}
		raws, e := rt.Session.Snapshot(ctx)
		if e != nil {
			panic(e)
		}
		snap, e := sanaei.InventoryFromRaw(*panel, raws)
		if e != nil {
			panic(e)
		}
		for _, r := range snap.Records {
			b, _ := json.Marshal(map[string]any{"inbound": r.RemoteID, "clients": r.ClientCount, "enabled": r.Enabled, "protocol": r.Protocol, "port": r.Port, "transport": r.Transport, "security": r.Security})
			fmt.Println(string(b))
		}
		return
	}
	var acc, did, host, user, keyref string
	e = a.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','') FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1`, *panel).Scan(&acc, &did, &host, &user, &keyref)
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
	if *routingContract {
		cmd = `strings /usr/local/x-ui/x-ui 2>/dev/null | awk '/^    "\/panel\/api\/xray\/routeTest":/ {p=1} p{if(n++>0 && /^    "\//)exit; print}' | head -180`
	}

	if *configTest {
		cmd = `set -eu
f=$(mktemp /tmp/dob-xray-validation.XXXXXX.json)
trap 'rm -f "$f"' EXIT INT TERM
cat >"$f" <<'XRAYJSON'
{"log":{"loglevel":"warning"},"inbounds":[],"outbounds":[{"tag":"dob-route-direct-test","protocol":"freedom","settings":{}},{"tag":"dob-route-blocked-test","protocol":"blackhole","settings":{}},{"tag":"residential-ads-test","protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":1080,"users":[{"user":"test","pass":"test"}]}]}}],"routing":{"rules":[{"type":"field","inboundTag":["in-443-tcp"],"user":["a@test"],"network":"tcp,udp","outboundTag":"dob-route-direct-test"},{"type":"field","inboundTag":["in-443-tcp"],"network":"tcp,udp","outboundTag":"residential-ads-test"}]}}
XRAYJSON
for bin in /usr/local/x-ui/bin/xray*; do
 if [ -f "$bin" ] && [ -x "$bin" ]; then "$bin" run -test -config "$f"; exit; fi
done
exit 1`
	}
	if *bulkDelete {
		cmd = `for f in /usr/local/x-ui/x-ui /usr/local/x-ui/bin/x-ui; do if [ -f "$f" ]; then strings "$f" 2>/dev/null | grep -A95 -B2 '"/panel/api/clients/bulkDel"' | head -98; fi; done`
	}
	if *clientRoutes {
		cmd = `for f in /usr/local/x-ui/x-ui /usr/local/x-ui/bin/x-ui; do if [ -f "$f" ]; then strings "$f" 2>/dev/null | grep -E '^    "/panel/api/clients/[^"]+":' | head -70; fi; done`
	}
	if *health {
		cmd = `systemctl is-active x-ui; ps -C x-ui -C xray -o comm=,rss=,%cpu=; free -m`
	}
	if *samples > 0 {
		cmd = fmt.Sprintf(`python3 - <<'PYRESOURCE'

import os,time,json,pathlib
root=pathlib.Path('/proc')
ticks=os.sysconf('SC_CLK_TCK')
prev={};last=time.monotonic();rows=[]
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
   elif index>0:cpu+=used/ticks/max(now-last,.001)*100
   for line in (p/'status').read_text().splitlines():
    if line.startswith('VmRSS:'):rss+=int(line.split()[1])
   procs+=1
  except (FileNotFoundError,ProcessLookupError):continue
 available=next(int(line.split()[1]) for line in (root/'meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))
 cg=pathlib.Path('/sys/fs/cgroup/system.slice/x-ui.service/memory.current')
 group=int(cg.read_text()) if cg.exists() else None
 rows.append(dict(time=time.time(),index=index,rss_kib=rss,cpu_pct=round(cpu,2) if index else None,processes=procs,available_kib=available,cgroup_memory_bytes=group))
 prev=current;last=now
 if index<%d:time.sleep(1)

print(json.dumps(dict(samples=len(rows),started_at=rows[0]['time'],ended_at=rows[-1]['time'],peak_sampled_rss_kib=max(x['rss_kib'] for x in rows),peak_sampled_cpu_percent=max(x['cpu_pct'] or 0 for x in rows),minimum_available_kib=min(x['available_kib'] for x in rows),peak_sampled_cgroup_bytes=max(x['cgroup_memory_bytes'] or 0 for x in rows),min_processes=min(x['processes'] for x in rows),max_processes=max(x['processes'] for x in rows),sampling_interval_seconds=1,first=rows[0],last=rows[-1])),flush=True)
PYRESOURCE`, *samples, *samples)
	}
	r, e := ssh.RunDetailed(ctx, t, key, cmd)
	fmt.Print(r.Stdout)
	if e != nil {
		fmt.Printf("ERR %v %s\n", e, r.Stderr)
	}
}
