#!/usr/bin/env python3
"""Fixed bounded traffic evidence. Every dispatched attempt has a durable journal."""
from pathlib import Path
from importlib.machinery import SourceFileLoader
import concurrent.futures
import ctypes
import datetime as dt
import hashlib
import json
import math
import os
import re
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.parse

support_path = Path(__file__).with_name("support")
if not support_path.exists():
    support_path = Path(__file__).with_name("residential-pilot-support.py")
support = SourceFileLoader("residential_traffic_support", str(support_path)).load_module()

def cases():
    result = []
    for roundno in range(3):
        for name, url in [
            ("ads", "https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js"),
            ("gpt", "https://securepubads.g.doubleclick.net/tag/js/gpt.js"),
            ("browserleaks", "https://browserleaks.com/ip"),
        ]:
            result.append((name + "-" + str(roundno), "RESIDENTIAL", url, 200, False))
    for host in ["www.gstatic.com", "connectivitycheck.gstatic.com", "www.google.com"]:
        result.append(("probe-" + host, "RESIDENTIAL", "https://" + host + "/generate_204", 204, False))
    for url in ["https://example.com/", "https://1.1.1.1/", "https://dns.google/dns-query"]:
        result.append(("deny-" + urllib.parse.urlsplit(url).hostname, "RESIDENTIAL", url, 0, False))
    return result + [
        ("direct-positive", "DIRECT", "https://example.com/", 200, False),
        ("egress-res", "RESIDENTIAL", "https://browserleaks.com/ip", 200, True),
        ("egress-direct", "DIRECT", "https://browserleaks.com/ip", 200, True),
    ]

def timestamp():
    return dt.datetime.now(dt.timezone.utc).isoformat()

class Evidence:
    def __init__(self, directory, panel, label):
        self.root, self.panel, self.label = Path(directory), panel, label
        self.journal = self.root / (label + ".events.jsonl")
        self.manifest = self.root / (label + ".manifest.json")
        if self.manifest.exists() or self.journal.exists():
            raise ValueError("traffic window already attempted")
        self.lock = threading.Lock()
        self.started, self.finished = {}, {}
        support.atomic(self.manifest, {
            "panel": panel, "phase": label, "created_at": timestamp(),
            "cases": [{"case": c[0], "class": c[1], "url": c[2], "expected_http": c[3]} for c in cases()],
        })
        support.create_journal(self.journal)
    def begin(self, case):
        value = {"event": "started", "case": case[0], "class": case[1], "at": timestamp()}
        with self.lock:
            support.append_event(self.journal, value)
            self.started[case[0]] = value
        return value["at"]
    def finish(self, value):
        with self.lock:
            support.append_event(self.journal, {"event": "finished", **value})
            self.finished[value["case"]] = value
    def finalize(self, core_healthy):
        with self.lock:
            ordered = []
            for c in cases():
                name = c[0]
                if name in self.finished:
                    ordered.append(self.finished[name])
                else:
                    ordered.append({"case": name, "class": c[1],
                                    "at": self.started.get(name, {}).get("at"),
                                    "outcome": "interrupted" if name in self.started else "not_started",
                                    "passed": False, "http": 0, "curl_code": -1,
                                    "seconds": 0, "egress_sha256": None})
            egress = {x["case"]: x["egress_sha256"] for x in ordered if x["egress_sha256"]}
            distinct = len(egress) == 2 and egress["egress-res"] != egress["egress-direct"]
            complete = len(self.finished) == 18
            report = {"at": timestamp(), "panel": self.panel, "phase": self.label,
                      "cases": ordered, "complete": complete, "egress_distinct": distinct,
                      "local_core_healthy": core_healthy,
                      "scope": "fixed actual RES/DIRECT traffic; not mobile latency or advertising events",
                      "passed": complete and all(x["passed"] for x in ordered) and distinct and core_healthy}
            support.atomic(self.root / (self.label + ".json"), report)
            return report

def attempt(case, port, directory, evidence, stopped):
    name, cls, url, expected, body = case
    if stopped.is_set():
        return None
    started = evidence.begin(case)
    elapsed = time.monotonic()
    result = {"case": name, "class": cls, "at": started, "outcome": "interrupted",
              "passed": False, "http": 0, "curl_code": -1, "seconds": 0, "egress_sha256": None}
    destination = Path(directory) / (name + ".body")
    args = ["curl", "--disable", "--retry", "0", "--silent", "--noproxy", "",
            "--socks5-hostname", "127.0.0.1:" + str(port), "--connect-timeout", "4",
            "--max-time", "6", "--output", str(destination), "--write-out", "%{json}"]
    if not body: args += ["--head"]
    try:
        response = subprocess.run(args + [url], capture_output=True, text=True, timeout=8)
        value = json.loads(response.stdout)
        http, seconds = value.get("http_code"), value.get("time_total")
        if type(http) is not int or type(seconds) not in (int, float) or not math.isfinite(seconds):
            raise ValueError("invalid curl metrics")
        passed = response.returncode == 0 and http == expected if expected else http == 0 and response.returncode in (35, 52, 56)
        result.update(outcome="completed", http=http, curl_code=response.returncode, seconds=seconds)
        if body and passed:
            match = re.search(r'id="client-ipv4"[^>]*data-ip="([^"]+)"', destination.read_text(errors="replace"))
            address = support.ipv4(match.group(1)) if match else None
            if address is None: raise ValueError("IPv4 egress evidence missing")
            result["egress_sha256"] = hashlib.sha256(address.encode()).hexdigest()
        result["passed"] = passed
    except Exception as error:
        result.update(outcome="interrupted" if isinstance(error, subprocess.TimeoutExpired) else "collector_error",
                      error_type=type(error).__name__, passed=False, seconds=time.monotonic() - elapsed)
    evidence.finish(result)
    return result

def freeport():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]

def run(panel, label, directory, binary):
    root = Path(directory)
    if not root.is_absolute() or not Path(binary).is_absolute() or not re.fullmatch(r"[a-f0-9-]{36}", panel) or not re.fullmatch(r"[a-z0-9-]+", label):
        raise ValueError("invalid bounded traffic identity")
    evidence = Evidence(root, panel, label)
    stopped = threading.Event()
    def stop(*unused):
        stopped.set()
        raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    core, executor, alive = None, None, False
    report = None
    try:
        sql = "SELECT COALESCE(jsonb_agg(t),'[]'::jsonb) FROM (SELECT DISTINCT ON(c.route_class) c.route_class,o.uri FROM output_config_snapshots o JOIN panel_client_routes c ON c.panel_id=o.panel_id AND split_part(split_part(o.uri,'@',1),'://',2)=c.client_id WHERE o.panel_id='" + panel + "' AND o.visible_until>clock_timestamp()+interval '180 seconds' AND o.last_seen_at>clock_timestamp()-interval '15 seconds' AND c.route_class IN('DIRECT','RESIDENTIAL') AND c.effective_class=c.route_class ORDER BY c.route_class,o.last_seen_at DESC,o.uri)t"
        with support.database_environment(root, os.environ) as env:
            response = subprocess.run(["psql", "-XAt", "-v", "ON_ERROR_STOP=1", "-c", sql],
                                      capture_output=True, text=True, env=env, timeout=12)
        if response.returncode: raise ValueError("fresh client read unavailable")
        rows = json.loads(response.stdout)
        if {x["route_class"] for x in rows} != {"DIRECT", "RESIDENTIAL"}:
            raise ValueError("fresh paired classes unavailable")
        inbounds, outbounds, rules, ports = [], [{"tag": "blocked", "protocol": "blackhole"}], [], {}
        for row in rows:
            cls = row["route_class"]
            uri = urllib.parse.urlsplit(row["uri"])
            query = urllib.parse.parse_qs(uri.query)
            def one(key, default=""): return query.get(key, [default])[0]
            if uri.scheme != "vless" or one("security") != "reality" or one("type", "tcp") != "tcp":
                raise ValueError("unsupported fixed client transport")
            ports[cls] = freeport()
            inbounds.append({"tag": cls, "listen": "127.0.0.1", "port": ports[cls], "protocol": "socks", "settings": {"auth": "noauth"}})
            outbounds.append({"tag": "tunnel-" + cls, "protocol": "vless", "settings": {"vnext": [{"address": uri.hostname, "port": uri.port, "users": [{"id": uri.username, "encryption": "none", "flow": one("flow")}]}]}, "streamSettings": {"network": "tcp", "security": "reality", "realitySettings": {"serverName": one("sni"), "fingerprint": one("fp", "chrome"), "publicKey": one("pbk"), "shortId": one("sid"), "spiderX": one("spx", "/")}}})
            rules.append({"type": "field", "inboundTag": [cls], "outboundTag": "tunnel-" + cls})
        with tempfile.TemporaryDirectory(prefix="pilot-", dir=root / "tmp") as temporary:
            config = Path(temporary) / "client.json"
            support.atomic(config, {"log": {"loglevel": "none"}, "inbounds": inbounds, "outbounds": outbounds, "routing": {"rules": rules}})
            parent = os.getpid()
            def child_setup():
                ctypes.CDLL(None).prctl(1, signal.SIGKILL)
                if os.getppid() != parent: os._exit(124)
            core = subprocess.Popen([binary, "run", "-config", str(config)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=child_setup)
            ready = False
            for _ in range(80):
                if core.poll() is not None: break
                try:
                    with socket.create_connection(("127.0.0.1", ports["RESIDENTIAL"]), timeout=.1): ready = True; break
                except OSError: time.sleep(.05)
            if not ready: raise ValueError("local traffic core did not become ready")
            executor = concurrent.futures.ThreadPoolExecutor(max_workers=2)
            futures = [executor.submit(attempt, case, ports[case[1]], temporary, evidence, stopped) for case in cases()]
            for future in concurrent.futures.as_completed(futures): future.result()
            alive = core.poll() is None
    except (Exception, KeyboardInterrupt):
        stopped.set()
        with evidence.lock:
            support.append_event(evidence.journal, {'event':'interrupted','at':timestamp(),'pending':sorted(set(evidence.started)-set(evidence.finished))})
        evidence.finalize(False)
    finally:
        if executor is not None: executor.shutdown(wait=True, cancel_futures=True)
        if core is not None and core.poll() is None:
            core.terminate()
            try: core.wait(timeout=1)
            except subprocess.TimeoutExpired: core.kill(); core.wait()
        report = evidence.finalize(alive)
    print(json.dumps({"phase": label, "passed": report["passed"], "complete": report["complete"],
                      "cases": len(report["cases"]), "failures": [x["case"] for x in report["cases"] if not x["passed"]]}))
    return 0 if report["passed"] else 1

if __name__ == "__main__":
    try: raise SystemExit(run(*sys.argv[1:5]))
    except Exception: print("traffic evidence incomplete; inspect private journal", file=sys.stderr); raise SystemExit(2)
