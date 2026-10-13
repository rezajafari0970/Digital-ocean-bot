#!/usr/bin/env python3
"""Owned canary only: synthetic data, loopback origins, bounded traffic, no panel writes."""
import argparse, concurrent.futures, hashlib, http.client, http.server, json, os, pathlib
import signal, socket, ssl, statistics, subprocess, sys, threading, time

def digest(p): return hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
def atom(path, value):
    path=pathlib.Path(path)
    temp=path.with_suffix(".pending")
    with temp.open("w") as f:
        json.dump(value,f,sort_keys=True); f.flush(); os.fsync(f.fileno())
    os.chmod(temp,0o600); os.replace(temp,path)
def proc_stop(p):
    if p and p.poll() is None:
        p.terminate()
        try: p.wait(timeout=4)
        except subprocess.TimeoutExpired: p.kill(); p.wait(timeout=2)
def wait_port(port,process):
    for _ in range(100):
        if process.poll() is not None: raise RuntimeError("owned_core_early_exit")
        try:
            with socket.create_connection(("127.0.0.1",port),.1): return
        except OSError: time.sleep(.05)
    raise RuntimeError("owned_core_listener_timeout")
def serve(root):
    root=pathlib.Path(root)
    if json.loads((root/"owner.json").read_text())["owner"]!="advanced-reality-20261013":
        raise RuntimeError("ownership_missing")
    payload=(b"are-owned-synthetic-"*16384)[:262144]
    class Origin(http.server.BaseHTTPRequestHandler):
        def log_message(self,*args): pass
        def do_GET(self):
            self.send_response(200); self.send_header("Content-Length",str(len(payload)));self.end_headers()
            try: self.wfile.write(payload)
            except (BrokenPipeError,ConnectionResetError): pass
        def do_POST(self):
            n=int(self.headers.get("Content-Length","0"))
            if n<0 or n>1048576: self.send_error(413);return
            raw=self.rfile.read(n); out=hashlib.sha256(raw).hexdigest().encode()
            self.send_response(200);self.send_header("Content-Length",str(len(out)));self.end_headers();self.wfile.write(out)
    class Quiet(http.server.ThreadingHTTPServer):
        daemon_threads=True
        def handle_error(self,*args): pass
    origin=Quiet(("127.0.0.1",18081),Origin)
    target=Quiet(("127.0.0.1",18443),Origin)
    tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);tls.minimum_version=ssl.TLSVersion.TLSv1_3
    tls.load_cert_chain(root/"cert.pem",root/"key.pem");tls.set_alpn_protocols(["h2","http/1.1"])
    target.socket=tls.wrap_socket(target.socket,server_side=True)
    for s in (origin,target): threading.Thread(target=s.serve_forever,daemon=True).start()
    stop=threading.Event()
    for sig in (signal.SIGINT,signal.SIGTERM): signal.signal(sig,lambda *a:stop.set())
    phase=None;p=None;generation=0;recoveries=0;started=time.monotonic()
    try:
        while not stop.is_set() and time.monotonic()-started<2400:
            desired=(root/"phase.txt").read_text().strip()
            if desired not in ("baseline","candidate","rollback"): raise RuntimeError("invalid_phase")
            if desired!=phase or (p and p.poll() is not None):
                crashed=p and p.poll() is not None and desired==phase
                if crashed:
                    recoveries+=1
                    if recoveries>2: raise RuntimeError("recovery_budget_exhausted")
                proc_stop(p)
                config=root/("candidate.json" if desired=="candidate" else "baseline.json")
                r=subprocess.run([str(root/"xray"),"run","-test","-config",str(config)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=10)
                if r.returncode: raise RuntimeError("native_validation_failed")
                p=subprocess.Popen([str(root/"xray"),"run","-config",str(config)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                for port in (24443,24444): wait_port(port,p)
                generation+=1;phase=desired
            status={"phase":phase,"generation":generation,"core_pid":p.pid,"core_config_sha256":digest(root/("candidate.json" if phase=="candidate" else "baseline.json")),
                    "recoveries":recoveries,"elapsed_seconds":round(time.monotonic()-started,2),"core_alive":p.poll() is None}
            try:
                raw=pathlib.Path("/proc",str(p.pid),"status").read_text().splitlines()
                status["core_memory_kib"]={line.split(":")[0]:int(line.split()[1]) for line in raw if line.startswith(("VmRSS:","VmHWM:"))}
                status["core_fds"]=len(list(pathlib.Path("/proc",str(p.pid),"fd").iterdir()))
            except OSError: pass
            atom(root/"state.json",status)
            stop.wait(.5)
    finally:
        proc_stop(p)
        origin.shutdown();target.shutdown();origin.server_close();target.server_close()
        atom(root/"stopped.json",{"owned_core_stopped":p is None or p.poll() is not None,"elapsed_seconds":round(time.monotonic()-started,2)})
def client(root,core,phase,requests_count):
    root=pathlib.Path(root); results=[]
    payload=(b"are-owned-synthetic-"*16384)[:262144];want=hashlib.sha256(payload).hexdigest()
    for mode,port in (("tcp",24801),("xhttp",24802)):
        path=root/("client-"+mode+".json")
        subprocess.run([core,"run","-test","-config",str(path)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True,timeout=10)
        p=subprocess.Popen([core,"run","-config",str(path)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        try:
            wait_port(port,p)
            def request(kind="download"):
                h=http.client.HTTPConnection("127.0.0.1",port,timeout=12);t=time.monotonic()
                try:
                    if kind=="upload":h.request("POST","http://127.0.0.1:18081/",body=payload)
                    else:h.request("GET","http://127.0.0.1:18081/")
                    response=h.getresponse()
                    if kind=="cancel": response.read(1024); return {"kind":kind,"ok":response.status==200,"ms":round((time.monotonic()-t)*1000,2)}
                    if kind=="slow":
                        body=b""
                        while True:
                            b=response.read(8192)
                            if not b:break
                            body+=b;time.sleep(.002)
                    else:body=response.read(len(payload)+1)
                    ok=response.status==200 and (body.decode()==want if kind=="upload" else hashlib.sha256(body).hexdigest()==want)
                    return {"kind":kind,"ok":ok,"bytes":len(body),"ms":round((time.monotonic()-t)*1000,2)}
                except Exception as e:return {"kind":kind,"ok":False,"error_type":type(e).__name__}
                finally:h.close()
            observations=[request(),request("upload"),request("slow")]
            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                observations+=list(pool.map(lambda _:request(),range(requests_count)))
            observations += [request("cancel") for _ in range(4)]
            observations += [request()]
            times=sorted(x["ms"] for x in observations if x.get("ok") and x["kind"]=="download")
            result={"phase":phase,"transport":mode,"core":subprocess.check_output([core,"version"],text=True).splitlines()[0],
                "requests":len(observations),"failed":sum(not x["ok"] for x in observations),"observations":observations,
                "download_p50_ms":statistics.median(times) if times else None,
                "download_p95_ms":times[min(len(times)-1,int(len(times)*.95))] if times else None}
            results.append(result)
            print(json.dumps({k:v for k,v in result.items() if k!="observations"}),flush=True)
        finally:proc_stop(p)
    atom(root/("result-"+phase+".json"),results)
    if any(x["failed"] for x in results): raise SystemExit(1)
if __name__=="__main__":
    a=argparse.ArgumentParser();a.add_argument("action",choices=("serve","client"));a.add_argument("--root",required=True)
    a.add_argument("--core");a.add_argument("--phase",default="baseline");a.add_argument("--requests",type=int,default=12)
    q=a.parse_args()
    if q.action=="serve":serve(q.root)
    else:
        if not 1<=q.requests<=32:raise SystemExit("bounded requests required")
        client(q.root,q.core,q.phase,q.requests)
