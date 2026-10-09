// residential-admission collects bounded operator-only evidence; it never mutates a panel.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	probe := flag.Bool("probe", false, "collect a bounded diagnostic receipt")
	panel := flag.String("panel", "", "one panel UUID")
	suspects := flag.String("suspects", "", "one or two comma-separated proxy UUIDs")
	controls := flag.String("controls", "", "two distinct control proxy UUIDs")
	binary := flag.String("xray", "", "absolute path to reviewed local Xray binary")
	evidence := flag.String("evidence", "", "read an existing receipt without probing")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *probe, *panel, *suspects, *controls, *binary, *evidence); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func ids(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, v := range strings.Split(s, ",") {
		v = strings.ToLower(strings.TrimSpace(v))
		if !residentialperf.UUID.MatchString(v) || seen[v] {
			return nil, errors.New("distinct UUIDs required")
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}
func run(parent context.Context, probe bool, panel, suspectArg, controlArg, binary, evidenceID string) error {
	if !probe && !residentialperf.UUID.MatchString(evidenceID) {
		return errors.New("use -evidence UUID for status or explicit -probe with fixed proxy roles")
	}
	var suspects, controls []string
	var err error
	if probe {
		suspects, err = ids(suspectArg)
		if err != nil {
			return err
		}
		controls, err = ids(controlArg)
		if err != nil {
			return err
		}
		if !residentialperf.UUID.MatchString(panel) || len(suspects) < 1 || len(suspects) > 2 || len(controls) != 2 || !filepath.IsAbs(binary) {
			return errors.New("probe requires one panel, 1-2 suspects, two controls and absolute local Xray path")
		}
		seen := map[string]bool{}
		for _, v := range append(append([]string{}, suspects...), controls...) {
			if seen[v] {
				return errors.New("probe roles must be distinct")
			}
			seen[v] = true
		}
	}
	lock, err := os.OpenFile("/opt/.digital-ocean-bot-deploy.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return errors.New("deployment lock unavailable")
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return errors.New("deployment in progress")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	ctx, cancel := context.WithTimeout(parent, 6*time.Minute)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return errors.New("service bootstrap unavailable")
	}
	defer a.Close()
	store := residentialperf.Store{DB: a.DB}
	if !probe {
		e, err := store.AdmissionEvidence(ctx, evidenceID)
		if err != nil {
			return errors.New("receipt unavailable")
		}
		return emit(e)
	}
	before, err := store.AdmissionContext(ctx, panel)
	if err != nil {
		return errors.New("current source context unavailable")
	}
	all := append(append([]string{}, suspects...), controls...)
	var uri string
	err = a.DB.QueryRowContext(ctx, `SELECT o.uri FROM output_config_snapshots o JOIN panel_client_routes c ON c.panel_id=o.panel_id
 AND split_part(split_part(o.uri,'@',1),'://',2)=c.client_id WHERE o.panel_id=$1 AND o.visible_until>clock_timestamp()+interval '600 seconds'
 AND o.last_seen_at>clock_timestamp()-interval '15 seconds' AND c.route_class='DIRECT' AND c.effective_class='DIRECT'
 ORDER BY o.last_seen_at DESC LIMIT 1`, panel).Scan(&uri)
	if err != nil {
		return errors.New("fresh DIRECT diagnostic tunnel unavailable")
	}
	tunnel, err := parseTunnel(uri)
	uri = ""
	if err != nil {
		return err
	}
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}}
	rt, err := manager.Acquire(ctx, panel)
	if err != nil {
		return errors.New("panel API unavailable")
	}
	response, err := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "GET", Path: "panel/api/server/getConfigJson", TimeoutSeconds: 8})
	if err != nil {
		return errors.New("native source configuration unavailable")
	}
	var top struct {
		Success bool
		Obj     json.RawMessage
	}
	if json.Unmarshal(response.Body, &top) != nil || !top.Success {
		return errors.New("invalid native configuration response")
	}
	var str string
	if json.Unmarshal(top.Obj, &str) == nil {
		top.Obj = json.RawMessage(str)
	}
	var native struct{ Outbounds []map[string]any }
	if json.Unmarshal(top.Obj, &native) != nil {
		return errors.New("native outbounds unavailable")
	}
	var outs []map[string]any
	for _, id := range all {
		if before.Proxies[id] < 1 {
			return errors.New("probe proxy is not currently enabled")
		}
		var host, user, tag, ref, typ string
		var port int
		err = a.DB.QueryRowContext(ctx, "SELECT host,port,COALESCE(username,''),outbound_tag,COALESCE(secret_ref,''),type FROM residential_proxies WHERE proxy_id=$1 AND enabled", id).Scan(&host, &port, &user, &tag, &ref, &typ)
		if err != nil || typ != "socks5" {
			return errors.New("collector requires enabled SOCKS5 endpoints")
		}
		password := []byte{}
		if ref != "" {
			password, err = a.Container.Secrets.GetResidential(ctx, id, ref)
			if err != nil {
				return errors.New("credential unavailable")
			}
		} else if user != "" {
			return errors.New("credential missing")
		}
		server := map[string]any{"address": host, "port": port}
		if user != "" || len(password) > 0 {
			server["users"] = []any{map[string]any{"user": user, "pass": string(password)}}
		}
		for i := range password {
			password[i] = 0
		}
		expected, _ := json.Marshal(map[string]any{"servers": []any{server}})
		found := false
		for _, out := range native.Outbounds {
			nativeTag, _ := out["tag"].(string)
			if nativeTag == tag || strings.HasPrefix(nativeTag, tag+"-") {
				got, _ := json.Marshal(out["settings"])
				if out["protocol"] == "socks" && string(got) == string(expected) {
					found = true
					break
				}
			}
		}
		if !found {
			return errors.New("native proxy configuration differs from current endpoint version")
		}
		outs = append(outs, map[string]any{"tag": "diagnostic-" + id, "protocol": "socks", "settings": map[string]any{"servers": []any{server}}, "streamSettings": map[string]any{"sockopt": map[string]any{"dialerProxy": "vps-diagnostic"}}})
	}
	e := residentialperf.AdmissionEvidence{Manifest: residentialperf.AdmissionManifest, Context: before, Suspects: suspects, Controls: controls, Started: time.Now().UTC()}
	e.ID, _ = sanaei.UUIDv4()
	for _, id := range all {
		for round := 0; round < 3; round++ {
			for _, target := range residentialperf.AdmissionTargets {
				e.Observations = append(e.Observations, residentialperf.AdmissionObservation{ProxyID: id, Target: target.Name, Round: round, Outcome: "not_started"})
			}
		}
	}
	ports, stop, coreAlive, err := startCoreTracked(ctx, binary, outs, tunnel)
	if err != nil {
		e.Guard.Startup = "diagnostic_core_unavailable"
		e.Finished = time.Now().UTC()
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
		defer finishCancel()
		if saveErr := store.RecordAdmission(finishCtx, e); saveErr != nil {
			return errors.New("startup failed and receipt persistence failed")
		}
		return emit(e)
	}
	defer stop()
	// Positive and blackholed-dialer controls prove the chain is necessary.
	positive := attempt(ctx, ports[len(suspects)], residentialperf.AdmissionGuardURL, 204)
	e.Guard.Positive = &positive
	e.Guard.Startup = "negative_core_unavailable"
	denyPorts, denyStop, denyAlive, denyErr := startCoreTracked(ctx, binary, outs[len(suspects):len(suspects)+1], map[string]any{"tag": "vps-diagnostic", "protocol": "blackhole"})
	if denyErr == nil {
		denied := attempt(ctx, denyPorts[0], residentialperf.AdmissionGuardURL, 204)
		e.Guard.Denied = &denied
		e.Guard.Startup = "ready"
		if conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(denyPorts[0])), time.Second); err == nil {
			conn.Close()
			e.Guard.NegativeCoreAlive = denyAlive()
		}
		denyStop()
		e.ChainVerified = e.Guard.Verified()
	}
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i := range all {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			for round := 0; round < 3; round++ {
				for j, target := range residentialperf.AdmissionTargets {
					if ctx.Err() != nil {
						return
					}
					o := attempt(ctx, ports[i], target.URL, target.Status)
					o.ProxyID = all[i]
					o.Target = target.Name
					o.Round = round
					e.Observations[i*12+round*4+j] = o
				}
			}
		}()
	}
	wg.Wait()
	e.CollectionHealthy = ctx.Err() == nil && coreAlive()
	e.CollectionOutcome = "completed"
	if !coreAlive() {
		e.CollectionOutcome = "diagnostic_core_exited"
	} else if ctx.Err() != nil {
		e.CollectionOutcome = "interrupted"
	}
	stop()
	e.Finished = time.Now().UTC()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
	defer finishCancel()
	after, err := store.AdmissionContext(finishCtx, panel)
	e.ContextStable = err == nil && before.Same(after)
	if err = store.RecordAdmission(finishCtx, e); err != nil {
		return errors.New("probe finished but receipt persistence failed")
	}
	return emit(e)
}
func emit(e residentialperf.AdmissionEvidence) error {
	reason := ""
	if err := e.Eligible(time.Now()); err != nil {
		reason = err.Error()
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"evidence": e, "eligible": reason == "", "reason": reason, "scope": "control host -> VLESS DIRECT -> residential SOCKS -> destination; separate from native pool and mobile latency"})
}
func parseTunnel(uri string) (map[string]any, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "vless" || u.User == nil {
		return nil, errors.New("invalid diagnostic transport")
	}
	q := u.Query()
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || q.Get("security") != "reality" || (q.Get("type") != "" && q.Get("type") != "tcp") || !residentialperf.UUID.MatchString(u.User.Username()) || q.Get("pbk") == "" {
		return nil, errors.New("unsupported diagnostic transport")
	}
	fp := q.Get("fp")
	if fp == "" {
		fp = "chrome"
	}
	spx := q.Get("spx")
	if spx == "" {
		spx = "/"
	}
	return map[string]any{"tag": "vps-diagnostic", "protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": u.Hostname(), "port": port, "users": []any{map[string]any{"id": u.User.Username(), "encryption": "none", "flow": q.Get("flow")}}}}}, "streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": q.Get("sni"), "fingerprint": fp, "publicKey": q.Get("pbk"), "shortId": q.Get("sid"), "spiderX": spx}}}, nil
}
func startCore(parent context.Context, binary string, outs []map[string]any, tunnel map[string]any) ([]int, func(), error) {
	ports, stop, _, err := startCoreTracked(parent, binary, outs, tunnel)
	return ports, stop, err
}
func startCoreTracked(parent context.Context, binary string, outs []map[string]any, tunnel map[string]any) ([]int, func(), func() bool, error) {
	dir, err := os.MkdirTemp("", "admission-")
	if err != nil {
		return nil, nil, nil, errors.New("private temporary directory unavailable")
	}
	cleanup := func() { os.RemoveAll(dir) }
	var ports []int
	var in, rules []any
	out := []any{map[string]any{"tag": "deny", "protocol": "blackhole"}}
	for i, o := range outs {
		l, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			cleanup()
			return nil, nil, nil, errors.New("local listener unavailable")
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		ports = append(ports, port)
		tag := fmt.Sprintf("in%d", i)
		in = append(in, map[string]any{"tag": tag, "listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}})
		rules = append(rules, map[string]any{"type": "field", "inboundTag": []string{tag}, "outboundTag": o["tag"]})
		out = append(out, o)
	}
	out = append(out, tunnel)
	config := map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": in, "outbounds": out, "routing": map[string]any{"rules": rules}}
	b, _ := json.Marshal(config)
	file := filepath.Join(dir, "core.json")
	if err = os.WriteFile(file, b, 0600); err != nil {
		cleanup()
		return nil, nil, nil, errors.New("private configuration unavailable")
	}
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, binary, "run", "-config", file)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err = cmd.Start(); err != nil {
		cancel()
		cleanup()
		return nil, nil, nil, errors.New("diagnostic Xray did not start")
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); <-done; cleanup() }) }
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return nil, nil, nil, errors.New("diagnostic startup interrupted")
		case <-done:
			stop()
			return nil, nil, nil, errors.New("diagnostic Xray exited")
		case <-deadline.C:
			stop()
			return nil, nil, nil, errors.New("diagnostic startup timed out")
		case <-tick.C:
			ready := true
			for _, port := range ports {
				c, e := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 50*time.Millisecond)
				if e != nil {
					ready = false
					break
				}
				c.Close()
			}
			if ready {
				return ports, stop, func() bool {
					select {
					case <-done:
						return false
					default:
						return true
					}
				}, nil
			}
		}
	}
}
func attempt(ctx context.Context, port int, target string, status int) residentialperf.AdmissionObservation {
	o := residentialperf.AdmissionObservation{Started: time.Now().UTC(), Outcome: "not_started"}
	if ctx.Err() != nil {
		o.Finished = o.Started
		return o
	}
	child, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(child, "curl", "--disable", "--retry", "0", "--silent", "--head", "--noproxy", "", "--socks5-hostname", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "--connect-timeout", "6", "--max-time", "10", "--output", "/dev/null", "--write-out", "%{json}", target)
	b, err := cmd.Output()
	o.Finished = time.Now().UTC()
	o.Milliseconds = o.Finished.Sub(o.Started).Milliseconds()
	var result struct {
		HTTPCode int `json:"http_code"`
	}
	json.Unmarshal(b, &result)
	o.HTTPStatus = result.HTTPCode
	o.Timing = parseTiming(b)
	if err == nil {
		if o.HTTPStatus == status {
			o.Outcome = "ok"
		} else {
			o.Outcome = "http"
		}
		return o
	}
	o.Outcome = "transport"
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		o.CurlCode = exit.ExitCode()
		switch exit.ExitCode() {
		case 28:
			o.Outcome = "timeout"
		case 7:
			o.Outcome = "local_proxy_unavailable"
		case 35, 60:
			o.Outcome = "tls"
		case 67:
			o.Outcome = "auth"
		}
	}
	if child.Err() != nil {
		o.CurlCode = -1
		o.Outcome = "interrupted"
	}
	return o
}

// Missing/invalid timing is left unknown; it never changes the transport verdict.
// URLs, addresses, headers, certificates and error strings are deliberately dropped.
func parseTiming(raw []byte) *residentialperf.AdmissionTiming {
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil {
		return nil
	}
	keys := []string{"time_namelookup", "time_connect", "time_appconnect", "time_pretransfer", "time_starttransfer", "time_total"}
	values := make([]float64, len(keys))
	for i, key := range keys {
		v, ok := data[key]
		if !ok || string(v) == "null" || json.Unmarshal(v, &values[i]) != nil || math.IsNaN(values[i]) || math.IsInf(values[i], 0) || values[i] < 0 || values[i] > 60 {
			return nil
		}
	}
	// Milestones may be zero when unfinished. Positive milestones cannot exceed
	// total time; no stronger ordering is inferred across curl/proxy versions.
	for _, v := range values[:5] {
		if v > values[5] {
			return nil
		}
	}
	if values[5] == 0 {
		return nil
	}
	return &residentialperf.AdmissionTiming{NameLookup: values[0], Connect: values[1], TLS: values[2], Pretransfer: values[3], FirstByte: values[4], Total: values[5]}
}
